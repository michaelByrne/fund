const plugin = require('tailwindcss/plugin')

/** @type {import('tailwindcss').Config} */
module.exports = {
    content: ["./**/*.html", "./**/*.templ", "./**/*.go",],
    theme: {
        extend: {
            colors: {
                'peach': '#ffd4a3',  // Define a custom color
            },
            textShadow: {
                DEFAULT: '1px 1px 0px var(--tw-shadow-color)',
            },
            boxShadow: {
                'blue-boxy': '5px 5px 0px 0px #788f99',
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