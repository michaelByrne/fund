window.paypal_once.Buttons({
    style: {
        shape: 'rect',
        color: 'blue',

    },
    createOrder: async function() {
        let fundId = JSON.parse(document.getElementById('fund-id').textContent)
        let amountCents = JSON.parse(document.getElementById('amount').textContent)

        let response = await fetch('/donation/once/initiate', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded'
            },
            body: new URLSearchParams({
                fund_id: fundId,
                amount_cents: amountCents
            })
        })

        let data = await response.json()

        return data.orderId
    },
    onApprove: async function(data, actions) {
        let capture = await actions.order.capture()
        let paymentId = capture.purchase_units[0].payments.captures[0].id

        let response = await fetch('/donation/once/complete', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded'
            },
            body: new URLSearchParams({
                order_id: data.orderID,
                amount: capture.purchase_units[0].amount.value,
                fund_id: JSON.parse(document.getElementById('fund-id').textContent),
                payment_id: paymentId
            })
        })

        if (response.ok) {
            window.location.href = '/donation/success?name=' + capture.payer.name.given_name
            return
        }

        let errResponseText = await response.text()

        handleDonationError(errResponseText)
    }
}).render('#paypal-button-container')

