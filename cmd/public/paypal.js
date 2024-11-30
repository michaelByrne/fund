window.paypal.Buttons({
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
        let bcoName = JSON.parse(document.getElementById('bco-name').textContent);

        let subscription = await actions.subscription.get();

        await htmx.ajax('POST','/subscription/capture', {
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded'
            },
            values: {
                order_id: data.orderID,
                provider_plan_id: providerPlanId,
                provider_donation_id: subscription.id,
                plan_id: planId,
                amount: subscription.billing_info.last_payment.amount.value,
                email: subscription.subscriber.email_address,
                payer_id: subscription.subscriber.payer_id,
                first_name: subscription.subscriber.name.given_name,
                last_name: subscription.subscriber.name.surname,
                bco_name: bcoName
            },
            target: "#donation"
        })
    }
}).render('#paypal-button-container')