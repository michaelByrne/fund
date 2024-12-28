const plugin = require('tailwindcss/plugin')

/** @type {import('tailwindcss').Config} */
module.exports = {
    content: ["./**/*.html", "./**/*.templ", "./**/*.go",],
    theme: {
        extend: {
            colors: {
                'contrast': 'rgb(var(--color-contrast), <alpha-value>)',
                'fore': 'rgb(var(--color-fore), <alpha-value>)',
                'even': 'rgb(var(--color-even), <alpha-value>)',
                'even-hover': 'rgb(var(--color-even-hover), <alpha-value>)',
                'odd': 'rgb(var(--color-odd), <alpha-value>)',
                'odd-hover': 'rgb(var(--color-odd-hover), <alpha-value>)',
                'high': 'rgb(var(--color-high), <alpha-value>)',
                'peak': 'rgb(var(--color-peak), <alpha-value>)',
                'back': 'rgb(var(--color-back), <alpha-value>)',
                'title': 'rgb(var(--color-title), <alpha-value>)',
                'links': 'rgb(var(--color-links), <alpha-value>)',
                'disabled': 'rgb(var(--color-disabled), <alpha-value>)',
                'accent': 'rgb(var(--color-accent), <alpha-value>)',
                'button-hover': 'rgb(var(--color-button-hover), <alpha-value>)',
                'button': 'rgb(var(--color-button), <alpha-value>)',
                'strong': 'rgb(var(--color-strong), <alpha-value>)',
            },
            textShadow: {
                DEFAULT: '1px 1px 0px var(--tw-shadow-color)',
            },
            boxShadow: {
                'blue-boxy': '5px 5px 0px 0px #788f99',
                'blue-boxy-thin': '3px 3px 0px 0px #788f99',
            },
            borderColor: {
                'top-peach': '#ffd4a3',  // Define a custom color
            },
        },
        fontFamily: {
            'bco': ['verdana', 'helvetica', 'arial', 'sans-serif'],
            'papyrus': ['papyrus', 'cursive'],
        }
    },
    plugins: [
        plugin(function ({matchUtilities, theme}) {
            matchUtilities(
                {
                    'text-shadow': (value) => ({
                        textShadow: value,
                    }),
                },
                {values: theme('textShadow')}
            )
        }),
    ],
}