const handleDonationError = (error) => {
    console.log(error)
    htmx.ajax('POST', '/error', {
        values: {
            error: error
        },
        headers: "Content-Type: application/x-www-form-urlencoded",
        target: "#donation",
    })
}