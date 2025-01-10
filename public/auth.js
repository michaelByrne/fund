document.addEventListener('alpine:init', () => {
    Alpine.data('auth', () => ({
        submitting: false,
        username: '',
        email: '',
        user: undefined,
        onSignUp: async function () {
            if (this.submitting) {
                return;
            }

            this.submitting = true;

            const formData = new FormData();
            formData.append('username', this.username);
            formData.append('email', this.email);

            try {
                const responseStart = await fetch('/auth/register', {
                    method: 'POST',
                    body: formData
                });

                if (!responseStart.ok) {
                    throw new Error(await responseStart.text());
                }

                const body = await responseStart.json();

                const registration = await SimpleWebAuthnBrowser.startRegistration({optionsJSON: body.publicKey});

                const responseFinish = await fetch('/auth/register', {
                    method: 'PUT',
                    body: JSON.stringify(registration),
                    headers: {
                        'Content-type': 'application/json'
                    }
                });

                if (!responseFinish.ok) {
                    throw new Error(await responseFinish.text());
                }

                window.location.href = '/auth/login';
            } catch (err) {
                console.log(err);
                alert(err.message);
            } finally {
                this.submitting = false;
            }
        },
        onSignIn: async function() {
            if (this.submitting) {
                return;
            }

            this.submitting = true;

            const formData = new FormData();
            formData.append('username', this.username);

            try {
                const responseStart = await fetch('/auth/login', {
                    method: 'POST',
                    body: formData
                });

                if (!responseStart.ok) {
                    throw new Error(await responseStart.text());
                }

                const body = await responseStart.json();

                const authentication = await SimpleWebAuthnBrowser.startAuthentication({optionsJSON: body.publicKey});

                const responseFinish = await fetch('/auth/login', {
                    method: 'PUT',
                    body: JSON.stringify(authentication),
                    headers: {
                        'Content-type': 'application/json'
                    }
                });

                if (!responseFinish.ok) {
                    throw new Error(await responseFinish.text());
                }

                window.location.href = '/';
            } catch (err) {
                console.log(err);
                alert(err.message);
            }
        }
    }));
});