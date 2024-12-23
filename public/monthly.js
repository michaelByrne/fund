const monthly = JSON.parse(me('#monthly').textContent);

new Chart(me(), {
    type: 'line',
    data: {
        datasets: [
            {
                label: 'Donation Amount',
                data: monthly.map(item => ({
                    x: item.month,
                    y: item.amount
                })),
                borderColor: '#8D99CB',
                yAxisID: 'y'
            },
            {
                label: 'Donors',
                data: monthly.map(item => ({
                    x: item.month,
                    y: item.unique_donors
                })),
                borderColor: '#FAA758',
                yAxisID: 'y1'
            }
        ],
    },
    options: {
        plugins: {
            legend: {
                display: true
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
                        if (context.dataset.yAxisID === 'y') {
                            const inDollars = context.raw.y / 100;
                            return inDollars.toLocaleString("en-US", {
                                style: "currency",
                                currency: "USD"
                            });
                        } else if (context.dataset.yAxisID === 'y1') {
                            return `${context.raw.y} donors`;
                        }
                    }
                }
            }
        },
        scales: {
            y: {
                position: 'left',
                ticks: {
                    callback: function (value) {
                        return (value / 100).toLocaleString("en-US", {
                            style: "currency",
                            currency: "USD"
                        });
                    }
                },
                title: {
                    display: true,
                    text: 'Donation Amount (USD)'
                }
            },
            y1: {
                position: 'right',
                grid: {
                    drawOnChartArea: false // Prevent gridlines overlapping with primary Y-axis
                },
                ticks: {
                    callback: function (value) {
                        return `${value}`;
                    }
                },
                title: {
                    display: true,
                    text: 'Donors'
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
            }
        }
    }
});