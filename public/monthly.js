const monthly = JSON.parse(me('#monthly').textContent);

new Chart(me(), {
    type: 'line',
    data: {
        datasets: [
            {
                data: monthly.map(item => ({
                    x: item.month,
                    y: item.amount
                }))
            }
        ],
    },
    options: {
        plugins: {
            legend: {
                display: false
            },
            tooltip: {
                callbacks: {
                    title: function (contexts) {
                        if (contexts.length > 0) {
                            const context = contexts[0];
                            const [year, month] = context.label.split("-");
                            return `${month}-${year}`;
                        }
                        return '';
                    },
                    label: function (context) {
                        const inDollars = context.raw.y / 100;
                        return inDollars.toLocaleString("en-US", {
                            style: "currency",
                            currency: "USD"
                        });
                    }
                }
            }
        },
        scales: {
            y: {
                ticks: {
                    callback: function (value) {
                        return (value / 100).toLocaleString("en-US", {style: "currency", currency: "USD"});
                    }
                }
            },
            x: {
                type: 'category',
                ticks: {
                    callback: function (value, index, values) {
                        if (index >= 0 && index < monthly.length) {
                            const [year, month] = monthly[index].month.split("-");
                            return `${month}-${year}`;
                        }
                        return '';
                    }
                }
            },
        }
    }
});
