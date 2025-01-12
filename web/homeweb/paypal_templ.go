// Code generated by templ - DO NOT EDIT.

// templ: version: v0.2.793
package homeweb

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/web/common"
	"fmt"
	"strings"
)

func PaypalSubscription(plan donations.DonationPlan, clientID, fundName string) templ.Component {
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
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<script src=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var2 string
		templ_7745c5c3_Var2, templ_7745c5c3_Err = templ.JoinStringErrs(fmt.Sprintf("https://www.paypal.com/sdk/js?client-id=%s&vault=true&intent=subscription", clientID))
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/homeweb/paypal.templ`, Line: 12, Col: 113}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var2))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\" data-namespace=\"paypal_sub\"></script><div class=\"payment-container w-[70%] blue-boxy-filter bg-even p-4\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.JSONScript("provider-plan-id", plan.ProviderPlanID).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.JSONScript("plan-id", plan.ID.String()).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.JSONScript("fund-id", plan.FundID.String()).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<h4 class=\"mb-2 mx-auto mt-2 text-xl p-2 font-papyrus font-semibold inline-block\">I am giving $")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var3 string
		templ_7745c5c3_Var3, templ_7745c5c3_Err = templ.JoinStringErrs(centsToDecimalString(plan.AmountCents))
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/homeweb/paypal.templ`, Line: 17, Col: 137}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var3))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(" every ")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var4 string
		templ_7745c5c3_Var4, templ_7745c5c3_Err = templ.JoinStringErrs(strings.ToLower(string(plan.IntervalUnit)))
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/homeweb/paypal.templ`, Line: 17, Col: 190}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var4))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(" to ")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var5 string
		templ_7745c5c3_Var5, templ_7745c5c3_Err = templ.JoinStringErrs(fundName)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/homeweb/paypal.templ`, Line: 17, Col: 206}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var5))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(".</h4><div id=\"paypal-button-container\"></div><script type=\"text/javascript\" src=\"/static/paypalsub.js\"></script></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func Paypal(fund donations.Fund, amountCents int32, clientID string) templ.Component {
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
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<script src=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var7 string
		templ_7745c5c3_Var7, templ_7745c5c3_Err = templ.JoinStringErrs(fmt.Sprintf("https://www.paypal.com/sdk/js?client-id=%s&vault=true", clientID))
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/homeweb/paypal.templ`, Line: 24, Col: 93}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var7))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\" data-namespace=\"paypal_once\"></script><div class=\"payment-container w-[70%] blue-boxy-filter bg-even p-4\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.JSONScript("fund-id", fund.ID.String()).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templ.JSONScript("amount", amountCents).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<h4 class=\"mb-2 mx-auto mt-2 text-xl font-papyrus p-2 font-semibold inline-block\">I am giving $")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var8 string
		templ_7745c5c3_Var8, templ_7745c5c3_Err = templ.JoinStringErrs(centsToDecimalString(amountCents))
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/homeweb/paypal.templ`, Line: 28, Col: 132}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var8))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(" to ")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var9 string
		templ_7745c5c3_Var9, templ_7745c5c3_Err = templ.JoinStringErrs(fund.Name)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/homeweb/paypal.templ`, Line: 28, Col: 149}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var9))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(".</h4><div id=\"paypal-button-container\"></div><script type=\"text/javascript\" src=\"/static/paypalonce.js\"></script></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func ThankYou(member members.Member, path string) templ.Component {
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
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div id=\"bouncing-elements\" style=\"position: relative;\"><div id=\"bouncing-element\" class=\"absolute font-semibold text-responsive\"><div class=\"inline-block\"><div class=\"blingy p-4 text-white font-papyrus\">Thank you ")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var12 string
			templ_7745c5c3_Var12, templ_7745c5c3_Err = templ.JoinStringErrs(member.FirstName)
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/homeweb/paypal.templ`, Line: 40, Col: 52}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var12))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("!!!</div></div></div><div id=\"bouncing-element2\" class=\"absolute font-semibold text-responsive-small\"><div class=\"bg-high p-4 text-blue-500 font-papyrus\"><a href=\"/\">donate more money</a></div></div></div><script>\n            function createBouncer(elementId, initialVelX, initialVelY) {\n                return {\n                    element: document.getElementById(elementId),\n                    pos: { x: 0, y: 0 },\n                    vel: { x: initialVelX, y: initialVelY },\n                    size: { width: 0, height: 0 }\n                };\n            }\n\n            const container = document.getElementById('donation');\n            const bouncer1 = createBouncer('bouncing-element', 2, 1.5);\n            const bouncer2 = createBouncer('bouncing-element2', 1.5, 2);\n            let containerSize = {};\n            let isAnimating = true;\n\n            function updateSizes() {\n                const rect = container.getBoundingClientRect();\n                containerSize = {\n                    width: rect.width,\n                    height: rect.height\n                };\n\n                [bouncer1, bouncer2].forEach(bouncer => {\n                    const elemRect = bouncer.element.getBoundingClientRect();\n                    bouncer.size = {\n                        width: elemRect.width,\n                        height: elemRect.height\n                    };\n                });\n\n                const baseVelocity = Math.min(containerSize.width, containerSize.height) / 400;\n                bouncer1.vel = {\n                    x: Math.sign(bouncer1.vel.x) * baseVelocity * 1.5,\n                    y: Math.sign(bouncer1.vel.y) * baseVelocity * 1.5\n                };\n                bouncer2.vel = {\n                    x: Math.sign(bouncer2.vel.x) * baseVelocity,\n                    y: Math.sign(bouncer2.vel.y) * baseVelocity\n                };\n            }\n\n            function adjustInitialPositions() {\n                [bouncer1, bouncer2].forEach(bouncer => {\n                    bouncer.pos.x = Math.min(\n                        containerSize.width - bouncer.size.width - 20,\n                        Math.max(20, bouncer.pos.x)\n                    );\n                    bouncer.pos.y = Math.min(\n                        containerSize.height - bouncer.size.height - 20,\n                        Math.max(20, bouncer.pos.y)\n                    );\n                });\n            }\n\n            function updateBouncer(bouncer) {\n                if (!isAnimating) return;\n\n                const nextX = bouncer.pos.x + bouncer.vel.x;\n                const nextY = bouncer.pos.y + bouncer.vel.y;\n\n                const dampening = 0.98;\n                const margin = 0;\n\n                if (nextX < margin) {\n                    bouncer.pos.x = margin;\n                    bouncer.vel.x = Math.abs(bouncer.vel.x) * dampening;\n                } else if (nextX + bouncer.size.width > containerSize.width - margin) {\n                    bouncer.pos.x = containerSize.width - bouncer.size.width - margin;\n                    bouncer.vel.x = -Math.abs(bouncer.vel.x) * dampening;\n                } else {\n                    bouncer.pos.x = nextX;\n                }\n\n                if (nextY < margin) {\n                    bouncer.pos.y = margin;\n                    bouncer.vel.y = Math.abs(bouncer.vel.y) * dampening;\n                } else if (nextY + bouncer.size.height > containerSize.height - margin) {\n                    bouncer.pos.y = containerSize.height - bouncer.size.height - margin;\n                    bouncer.vel.y = -Math.abs(bouncer.vel.y) * dampening;\n                } else {\n                    bouncer.pos.y = nextY;\n                }\n\n                const minVel = 0.2;\n                const maxVel = 2;\n                bouncer.vel.x = Math.min(maxVel, Math.max(minVel, Math.abs(bouncer.vel.x))) * Math.sign(bouncer.vel.x);\n                bouncer.vel.y = Math.min(maxVel, Math.max(minVel, Math.abs(bouncer.vel.y))) * Math.sign(bouncer.vel.y);\n\n                bouncer.element.style.transform = `translate(${bouncer.pos.x}px, ${bouncer.pos.y}px)`;\n            }\n\n            function animate() {\n                updateBouncer(bouncer1);\n                updateBouncer(bouncer2);\n                requestAnimationFrame(animate);\n            }\n\n            let resizeTimeout;\n            window.addEventListener('resize', () => {\n                clearTimeout(resizeTimeout);\n                isAnimating = false;\n\n                resizeTimeout = setTimeout(() => {\n                    updateSizes();\n                    adjustInitialPositions();\n                    isAnimating = true;\n                }, 100);\n            });\n\n            // Initial setup with a delay to ensure proper size calculation\n            setTimeout(() => {\n                updateSizes();\n                adjustInitialPositions();\n                requestAnimationFrame(animate);\n            }, 200);\n\n            // Pause animation when tab is not visible\n            document.addEventListener('visibilitychange', () => {\n                isAnimating = !document.hidden;\n            });\n        </script>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			return templ_7745c5c3_Err
		})
		templ_7745c5c3_Err = common.Layout(&member, path).Render(templ.WithChildren(ctx, templ_7745c5c3_Var11), templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

var _ = templruntime.GeneratedTemplate
