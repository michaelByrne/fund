const handleDonationError = (error) => {
    htmx.ajax('POST', '/error', {
        values: {
            error: error
        },
        headers: "Content-Type: application/x-www-form-urlencoded",
        target: "#donation",
    })
}

document.addEventListener("htmx:confirm", function (e) {
    // The event is triggered on every trigger for a request, so we need to check if the element
    // that triggered the request has a hx-confirm attribute, if not we can return early and let
    // the default behavior happen
    if (!e.detail.target.hasAttribute('hx-confirm')) return

    // This will prevent the request from being issued to later manually issue it
    e.preventDefault()

    Swal.fire({
        text: e.detail.question,
        background: '#FFE8D3',
        confirmButtonColor: "#D8E0FF",
        confirmButtonText: "<span style='color: #333333'>do it</span>",
        position: 'top',
        showCancelButton: true,
        cancelButtonText: "<span style='color: #333333'>nah</span>",
        cancelButtonColor: "#D8E0FF",
        customClass: {
            confirmButton: 'deactivate-popup-button',
            cancelButton: 'deactivate-popup-button',
            popup: 'deactivate-modal',
        }
    }).then(function (result) {
        if (result.isConfirmed) {
            // If the user confirms, we manually issue the request
            e.detail.issueRequest(true); // true to skip the built-in window.confirm()
        }
    })
})

document.addEventListener('htmx:responseError', evt => {
    const xhr = evt.detail.xhr;

    if (xhr.status == 422) {
        const errors = JSON.parse(xhr.responseText);

        for (const formId of Object.keys(errors)) {
            const formErrors = errors[formId];

            for (const name of Object.keys(formErrors)) {
                const field = document.querySelector(`#${formId} [name="${name}"]`);

                field.setCustomValidity(formErrors[name]);
                field.addEventListener('focus', () => field.reportValidity());
                field.addEventListener('change', () => field.setCustomValidity(''));
                field.reportValidity();
            }
        }
    }
});

let currentDropdown = null;

function toggleDropdown(event, iconElement) {
    event.preventDefault();
    event.stopPropagation();

    const dropdown = iconElement.nextElementSibling;

    if (currentDropdown === dropdown) {
        closeCurrentDropdown();
        return;
    }

    closeCurrentDropdown();

    dropdown.classList.remove('hidden');
    currentDropdown = dropdown;

    htmx.trigger(iconElement, 'audit-load');

    document.addEventListener('click', closeDropdownOnClickOutside);
}

function closeCurrentDropdown() {
    if (currentDropdown) {
        currentDropdown.classList.add('hidden');
        document.removeEventListener('click', closeDropdownOnClickOutside);
        currentDropdown = null;
    }
}

function closeDropdownOnClickOutside(event) {
    if (currentDropdown && !currentDropdown.contains(event.target) && !event.target.matches('.gear-icon')) {
        closeCurrentDropdown();
    }
}

document.addEventListener('htmx:beforeCleanupElement', function(evt) {
    //closeCurrentDropdown();
});