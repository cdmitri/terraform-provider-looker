package provider

import (
	"context"
	"github.com/devoteamgcloud/terraform-provider-looker/pkg/lookergo"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"strconv"
	"time"
	"strings"
)

// -
func resourceUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceUserCreate,
		ReadContext:   resourceUserRead,
		UpdateContext: resourceUserUpdate,
		DeleteContext: resourceUserDelete,
		Schema: map[string]*schema.Schema{
			"last_updated": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"id": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"first_name": &schema.Schema{
				Type:         schema.TypeString,
				Computed:     false,
				Optional:     true,
				ValidateFunc: validation.StringLenBetween(1, 255),
			},
			"last_name": &schema.Schema{
				Type:         schema.TypeString,
				Computed:     false,
				Optional:     true,
				ValidateFunc: validation.StringLenBetween(1, 255),
			},
			"email": &schema.Schema{
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringLenBetween(0, 255),
			},
			"roles": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"already_exists_ok": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
			},
			"delete_on_destroy": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
			},
		},
		Importer: &schema.ResourceImporter{
			// State: schema.ImportStatePassthrough,
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

func checkUserAlreadyExists(ctx context.Context, d *schema.ResourceData, c *lookergo.Client, email string) (lookergo.User, error) {
	users, _, err := c.Users.ListByEmail(ctx, email, &lookergo.ListOptions{})
	if err != nil {
		return lookergo.User{}, err
	}
	for _, user := range users {
		if strings.EqualFold(user.CredentialEmail.Email, email) {
			return user, nil
		}
	} 
	return lookergo.User{}, nil
}

func resourceUserCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Config).Api // .(*lookergo.Client)
	var diags diag.Diagnostics

	if d.Get("already_exists_ok") == true {
		if d.Get("email") != nil {
			user, err := checkUserAlreadyExists(ctx, d, c, d.Get("email").(string))
			if err != nil {
				return diag.FromErr(err)
			}
			if user.Id > 0{
				d.SetId(strconv.Itoa(user.Id))
				resourceUserRead(ctx, d, m)
				return diags
			}
		}
	}
	tflog.Info(ctx, "Creating Looker user")
	// Warning or errors can be collected in a slice type

	var userOptions = lookergo.User{
		FirstName: d.Get("first_name").(string),
		LastName:  d.Get("last_name").(string),
	}

	newUser, _, err := c.Users.Create(ctx, &userOptions)
	newEmail := new(lookergo.CredentialEmail)
	if err != nil {
		return diag.FromErr(err)
	}

	if d.Get("email").(string) != "" {
		emailOptions := lookergo.CredentialEmail{Email: d.Get("email").(string), IsDisabled: false}

		newEmail, _, err = c.Users.CreateEmail(ctx, newUser.Id, &emailOptions)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	roles := d.Get("roles").(*schema.Set)
	var newRoles []lookergo.Role
	if roles.Len() >= 1 {
		var r []string
		for _, role := range roles.List() {
			i, _ := strconv.Atoi(role.(string))
			r = append(r, idAsString(i))
		}

		newRoles, _, err = c.Users.SetRoles(ctx, newUser.Id, r)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId(strconv.Itoa(newUser.Id))

	resourceUserRead(ctx, d, m)
	tflog.Info(ctx, "Created Looker user", map[string]interface{}{"user": newUser, "email": newEmail, "roles": newRoles})

	return diags
}

func resourceUserRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Config).Api // .(*lookergo.Client)
	var diags diag.Diagnostics
	if d.Get("already_exists_ok") == true {
		user, _, err := c.Users.Get(ctx, idAsInt(d.Id()))
		if err != nil {
			return diag.FromErr(err)
		}
		d.Set("email", user.CredentialEmail.Email)
		d.Set("first_name", user.FirstName)
		d.Set("last_name", user.LastName)
		d.Set("roles", user.RoleIds.ToSliceOfStrings())
		return diags
	}
	userID := idAsInt(d.Id())

	user, _, err := c.Users.Get(ctx, userID)
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("id", strconv.Itoa(user.Id)); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("first_name", user.FirstName); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("last_name", user.LastName); err != nil {
		return diag.FromErr(err)
	}
	if user.RoleIds != nil {
		err = d.Set("roles", user.RoleIds.ToSliceOfStrings())
		if err != nil {
			return diag.FromErr(err)
		}
	}

	email, _, err := c.Users.GetEmail(ctx, userID)
	if err != nil {
		return diags
	} else if email != nil {
		if err = d.Set("email", email.Email); err != nil {
			return diag.FromErr(err)
		}
	}

	return diags
}

func resourceUserUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Config).Api // .(*lookergo.Client)
	userID, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	userOptions, _, err := c.Users.Get(ctx, userID)
	if err != nil {
		return diag.FromErr(err)
	}

	userOptions.LastName = d.Get("last_name").(string)
	userOptions.FirstName = d.Get("first_name").(string)

	_, _, err = c.Users.Update(ctx, userID, userOptions)
	if err != nil {
		return diag.FromErr(err)
	}

	if d.HasChange("email") {
		if d.Get("email").(string) == "" {
			_, err = c.Users.DeleteEmail(ctx, userID)
			if err != nil {
				return diag.FromErr(err)
			}
		} else {
			emailOptions := lookergo.CredentialEmail{Email: d.Get("email").(string)}

			_, _, err := c.Users.UpdateEmail(ctx, userID, &emailOptions)
			if err != nil {
				return diag.FromErr(err)
			}
		}
	}

	_ = d.Set("last_updated", time.Now().Format(time.RFC850))

	return resourceUserRead(ctx, d, m)
}

func resourceUserDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Config).Api // .(*lookergo.Client)
	//
	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	userID, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	
	if d.Get("delete_on_destroy") == true {
		_, err = c.Users.Delete(ctx, userID)
		if err != nil {
			return diag.FromErr(err)
		}
		email, _, err := c.Users.GetEmail(ctx, userID)
		if err != nil {
			return diags
		} else if email != nil {
			_, err = c.Users.DeleteEmail(ctx, userID)
			if err != nil {
				return diag.FromErr(err)
			}
		}
		d.SetId("")
	} else {
		d.SetId("")
	}
	return diags
}