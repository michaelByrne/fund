window.paypal_sub.Buttons({
    style: {
        shape: 'rect',
        color: 'blue',
    },
    createSubscription: function (data, actions) {
        let providerPlanId = JSON.parse(document.getElementById('provider-plan-id').textContent);
        return actions.subscription.create({
            'plan_id': providerPlanId,
            'application_context': {
                'locale': 'en-US',
                'shipping_preference': 'NO_SHIPPING',
                'user_action': 'SUBSCRIBE_NOW',
                'payment_method': {
                    'payer_selected': 'PAYPAL',
                    'payee_preferred': 'IMMEDIATE_PAYMENT_REQUIRED'
                }
            }
        });
    },
    onApprove: async function (data, actions) {
        let providerPlanId = JSON.parse(document.getElementById('provider-plan-id').textContent);
        let planId = JSON.parse(document.getElementById('plan-id').textContent);
        let fundId = JSON.parse(document.getElementById('fund-id').textContent);

        let subscription = await actions.subscription.get();

        let resp = await fetch('/donation/plan/complete', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded'
            },
            body: new URLSearchParams({
                order_id: data.orderID,
                provider_plan_id: providerPlanId,
                provider_donation_id: subscription.id,
                subscription_id: subscription.id,
                plan_id: planId,
                amount: subscription.billing_info.last_payment.amount.value,
                email: subscription.subscriber.email_address,
                payer_id: subscription.subscriber.payer_id,
                first_name: subscription.subscriber.name.given_name,
                last_name: subscription.subscriber.name.surname,
                fund_id: fundId
            })
        });

        if (resp.ok) {
            window.location.href = '/donation/success?name=' + subscription.subscriber.name.given_name;
        }
    }
}).render('#paypal-button-container')