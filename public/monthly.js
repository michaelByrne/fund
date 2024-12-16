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
                    label: function (context) {
                        const [year, month] = context.label.split("-");
                        let inDollars = context.parsed.y / 100;
                        return `${month}-${year}: ${inDollars.toLocaleString("en-US", {
                            style: "currency",
                            currency: "USD"
                        })}`;
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
        },
    }
});
