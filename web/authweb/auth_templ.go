// Code generated by templ - DO NOT EDIT.

// templ: version: v0.2.793
package authweb

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import "boardfund/web/common"

func Password() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var1 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var1 == nil {
			templ_7745c5c3_Var1 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Var2 := templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
			templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
			templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
			if !templ_7745c5c3_IsBuffer {
				defer func() {
					templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
					if templ_7745c5c3_Err == nil {
						templ_7745c5c3_Err = templ_7745c5c3_BufErr
					}
				}()
			}
			ctx = templ.InitializeContext(ctx)
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"md:w-[50%] w-[75%] mx-auto mt-4 pt-4 flex bg-high shadow-blue-boxy items-center\"><form action=\"/password\" method=\"post\" class=\"w-full\"><div class=\"flex flex-col gap-6\"><!-- Username Field --><div class=\"flex flex-col items-center gap-2\"><label for=\"username\" class=\"text-md font-semibold\">username</label> <input type=\"text\" name=\"username\" id=\"username\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><!-- Old Password Field --><div class=\"flex flex-col items-center gap-2\"><label for=\"old\" class=\"text-md font-semibold\">old password</label> <input type=\"password\" name=\"old\" id=\"old\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><!-- New Password Field --><div class=\"flex flex-col items-center gap-2\"><label for=\"new\" class=\"text-md font-semibold\">new password</label> <input type=\"password\" name=\"new\" id=\"new\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><!-- Confirm New Password Field --><div class=\"flex flex-col items-center gap-2\"><label for=\"confirm\" class=\"text-md font-semibold\">confirm new</label> <input type=\"password\" name=\"confirm\" id=\"confirm\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><!-- Submit Button --><div class=\"flex justify-center mt-4\"><button type=\"submit\" class=\"px-6 py-3 text-md font-medium bg-button text-black hover:shadow-blue-boxy-thin shadow-blue-boxy\">reset</button></div></div></form></div>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			return templ_7745c5c3_Err
		})
		templ_7745c5c3_Err = common.Layout(nil, "/password").Render(templ.WithChildren(ctx, templ_7745c5c3_Var2), templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func PasswordRegistration() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var3 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var3 == nil {
			templ_7745c5c3_Var3 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Var4 := templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
			templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
			templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
			if !templ_7745c5c3_IsBuffer {
				defer func() {
					templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
					if templ_7745c5c3_Err == nil {
						templ_7745c5c3_Err = templ_7745c5c3_BufErr
					}
				}()
			}
			ctx = templ.InitializeContext(ctx)
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"md:w-[50%] w-[75%] mx-auto mt-4 pt-4 flex bg-high shadow-blue-boxy items-center\"><form hx-post=\"/register\" class=\"w-full\"><div class=\"flex flex-col gap-6\"><div class=\"flex flex-col items-center gap-2\"><label for=\"username\" class=\"text-md font-semibold\">username</label> <input type=\"text\" name=\"username\" id=\"username\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><div class=\"flex flex-col items-center gap-2\"><label for=\"email\" class=\"text-md font-semibold\">email</label> <input type=\"text\" name=\"email\" id=\"email\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><div class=\"flex flex-col items-center justify-center\"><button type=\"submit\" class=\"px-6 py-3 text-md font-medium bg-button text-black hover:shadow-blue-boxy-thin shadow-blue-boxy\">register</button></div></div></form></div>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			return templ_7745c5c3_Err
		})
		templ_7745c5c3_Err = common.Layout(nil, "/register").Render(templ.WithChildren(ctx, templ_7745c5c3_Var4), templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func RegistrationSuccess() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var5 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var5 == nil {
			templ_7745c5c3_Var5 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex flex-col items-center gap-4 p-4\"><h1 class=\"text-2xl font-semibold\">registration successful</h1><p class=\"text-md text-center\">your account has been created. please check your email for a temporary password.</p></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func Registration() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var6 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var6 == nil {
			templ_7745c5c3_Var6 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Var7 := templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
			templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
			templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
			if !templ_7745c5c3_IsBuffer {
				defer func() {
					templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
					if templ_7745c5c3_Err == nil {
						templ_7745c5c3_Err = templ_7745c5c3_BufErr
					}
				}()
			}
			ctx = templ.InitializeContext(ctx)
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"md:w-[50%] w-[75%] mx-auto mt-4 pt-4 flex bg-high shadow-blue-boxy\"><form @submit.prevent=\"onSignUp()\" class=\"w-full flex flex-col items-center gap-4\"><div class=\"flex flex-col items-center gap-2\"><div class=\"flex flex-col items-center gap-2\"><label for=\"username\" class=\"text-md font-semibold\">username</label> <input x-model=\"username\" type=\"text\" name=\"username\" id=\"username\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"> <label for=\"email\" class=\"text-md font-semibold\">email</label> <input x-model=\"email\" type=\"text\" name=\"email\" id=\"email\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><button type=\"submit\" class=\"px-6 py-3 text-md font-medium bg-button text-black hover:shadow-blue-boxy-thin shadow-blue-boxy\">register</button></div></form></div>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			return templ_7745c5c3_Err
		})
		templ_7745c5c3_Err = common.Layout(nil, "/auth/register").Render(templ.WithChildren(ctx, templ_7745c5c3_Var7), templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func PasskeyLogin() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var8 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var8 == nil {
			templ_7745c5c3_Var8 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Var9 := templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
			templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
			templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
			if !templ_7745c5c3_IsBuffer {
				defer func() {
					templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
					if templ_7745c5c3_Err == nil {
						templ_7745c5c3_Err = templ_7745c5c3_BufErr
					}
				}()
			}
			ctx = templ.InitializeContext(ctx)
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"md:w-[50%] w-[75%] mx-auto mt-4 pt-4 flex bg-high shadow-blue-boxy\"><form @submit.prevent=\"onSignIn()\" class=\"w-full flex flex-col items-center gap-4\"><div class=\"flex flex-col items-center gap-2\"><label for=\"username\" class=\"text-md font-semibold\">username</label> <input x-model=\"username\" type=\"text\" name=\"username\" id=\"username\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><button type=\"submit\" class=\"px-6 py-3 text-md font-medium bg-button text-black hover:shadow-blue-boxy-thin shadow-blue-boxy\">login</button></form></div>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			return templ_7745c5c3_Err
		})
		templ_7745c5c3_Err = common.Layout(nil, "/auth/login").Render(templ.WithChildren(ctx, templ_7745c5c3_Var9), templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func Login() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var10 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var10 == nil {
			templ_7745c5c3_Var10 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Var11 := templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
			templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
			templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
			if !templ_7745c5c3_IsBuffer {
				defer func() {
					templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
					if templ_7745c5c3_Err == nil {
						templ_7745c5c3_Err = templ_7745c5c3_BufErr
					}
				}()
			}
			ctx = templ.InitializeContext(ctx)
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"md:w-[50%] w-[75%] mx-auto mt-4 pt-4 flex bg-high shadow-blue-boxy items-center\"><form action=\"/login\" target=\"_self\" method=\"post\" class=\"w-full\"><div class=\"flex flex-col gap-6\"><div class=\"flex flex-col items-center gap-2\"><label for=\"username\" class=\"text-md font-semibold\">username</label> <input type=\"text\" name=\"username\" id=\"username\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><div class=\"flex flex-col items-center gap-2\"><label for=\"password\" class=\"text-md font-semibold\">password</label> <input type=\"password\" name=\"password\" id=\"password\" required class=\"w-[80%] max-w-xs text-sm border border-slate-300 shadow-sm px-3 py-2\"></div><div class=\"flex flex-col md:flex-row items-center justify-center\"><button type=\"submit\" class=\"px-6 py-3 text-md font-medium bg-button text-black hover:shadow-blue-boxy-thin shadow-blue-boxy\">login</button> <a href=\"/register\" class=\"absolute right-[29%] lg:right-[35%] sm:visible invisible text-xs text-blue-400 hover:text-blue-800\">register</a></div></div></form></div>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			return templ_7745c5c3_Err
		})
		templ_7745c5c3_Err = common.Layout(nil, "/login").Render(templ.WithChildren(ctx, templ_7745c5c3_Var11), templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

var _ = templruntime.GeneratedTemplate
