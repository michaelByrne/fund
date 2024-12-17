document.addEventListener('htmx:afterRequest', function (event) {
    if (event.detail.requestConfig.path === '/donation/once') {
        const response = JSON.parse(event.detail.xhr.responseText);

        const popup = window.open(response.approvalUrl, 'PayPalAuth', 'width=600,height=600');

        const interval = setInterval(() => {
            if (popup.closed) {
                clearInterval(interval);

                console.log('Popup closed, handle post-payment actions here.');
            }
        }, 500);
    }
});