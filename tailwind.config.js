/** @type {import('tailwindcss').Config} */
module.exports = {
    content: [ "./**/*.html", "./**/*.templ", "./**/*.go", ],
    theme: {
        extend: {},
        fontFamily: {
            'bco': ['verdana', 'helvetica', 'arial', 'sans-serif'],
        }
    },
    plugins: [],
}