const { fontFamily } = require('tailwindcss/defaultTheme');

/** @type {import('tailwindcss').Config} */

module.exports = {
  darkMode: ['selector'],
  content: [
    './src/pages/**/*.tsx',
    './src/components/**/*.tsx',
    './src/layouts/**/*.tsx',
  ],
  theme: {
    container: {
      center: true,
      padding: '2rem',
      screens: {
        '2xl': '1400px',
      },
    },
    extend: {
      colors: {
        border: 'var(--colors-outline-neutral-strong)',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        background: 'var(--background)',
        foreground: 'var(--colors-text-neutral-strong)',
        buttonBlueText: 'var(--button-blue-text)',

        'colors-outline-sentiment-primary':
          'var(--colors-outline-sentiment-primary)',
        'colors-outline-neutral-strong': 'var(--colors-outline-neutral-strong)',
        'colors-outline-neutral-standard':
          'var(--colors-outline-neutral-standard)',

        'colors-text-core-standard': 'var(--colors-text-core-standard)',
        'colors-text-neutral-strong': 'var(--colors-text-neutral-strong)',
        'colors-text-neutral-standard': 'var(--colors-text-neutral-standard)',
        'colors-text-functional-danger': 'var(--colors-text-functional-danger)',
        'colors-text-inverse-strong': 'var(--colors-text-inverse-strong)',

        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))',
        },
        secondary: {
          DEFAULT: 'var(--background-inverse-strong)',
          foreground: 'var(--background-inverse-strong-foreground)',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))',
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))',
        },
        popover: {
          DEFAULT: 'hsl(var(--popover))',
          foreground: 'hsl(var(--popover-foreground))',
        },
        card: {
          DEFAULT: 'var(--background-inverse-standard)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        backgroundInverseStandard: {
          DEFAULT: 'var(--background-inverse-standard)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        backgroundInverseWeak: {
          DEFAULT: 'var(--background-inverse-weak)',
          foreground: 'var(--background-inverse-weak-foreground)',
        },
        backgroundCoreStandard: {
          DEFAULT: 'var(--background-core-standard)',
          foreground: 'var(--background-core-standard-foreground)',
        },
        backgroundCoreWeak: {
          DEFAULT: 'var(--background-core-weak)',
          foreground: 'var(--background-core-weak-foreground)',
        },

        'colors-background-inverse-standard': {
          DEFAULT: 'var(--colors-background-inverse-standard)',
          foreground: 'var(--colors-background-inverse-standard-foreground)',
        },

        'color-background-brand-default': {
          DEFAULT: 'var(--color-background-brand-default)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'color-background-positive-default': {
          DEFAULT: 'var(--color-background-positive-default)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-core-standard': {
          DEFAULT: 'var(--colors-background-core-standard)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-core-strong': {
          DEFAULT: 'var(--colors-background-core-strong)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-core-weak': {
          DEFAULT: 'var(--colors-background-core-weak)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-functional-solid-danger': {
          DEFAULT: 'var(--colors-background-functional-solid-danger)',
          foreground: 'var(--colors-text-inverse-strong)',
        },
        'colors-background-functional-solid-notice': {
          DEFAULT: 'var(--colors-background-functional-solid-notice)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-functional-solid-positive': {
          DEFAULT: 'var(--colors-background-functional-solid-positive)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-functional-transparent-danger': {
          DEFAULT: 'var(--colors-background-functional-transparent-danger)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-functional-transparent-notice': {
          DEFAULT: 'var(--colors-background-functional-transparent-notice)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-functional-transparent-positive': {
          DEFAULT: 'var(--colors-background-functional-transparent-positive)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-inverse-standard': {
          DEFAULT: 'var(--colors-background-inverse-standard)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-inverse-strong': {
          DEFAULT: 'var(--colors-background-inverse-strong)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-inverse-weak': {
          DEFAULT: 'var(--colors-background-inverse-weak)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-neutral-standard': {
          DEFAULT: 'var(--colors-background-neutral-standard)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-neutral-strong': {
          DEFAULT: 'var(--colors-background-neutral-strong)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-neutral-weak': {
          DEFAULT: 'var(--colors-background-neutral-weak)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
      },
      borderRadius: {
        lg: `var(--radius)`,
        md: `calc(var(--radius) - 2px)`,
        sm: 'calc(var(--radius) - 4px)',
      },
      fontFamily: {
        sans: ['var(--font-sans)', ...fontFamily.sans],
      },
      keyframes: {
        'accordion-down': {
          from: { height: '0' },
          to: { height: 'var(--radix-accordion-content-height)' },
        },
        'accordion-up': {
          from: { height: 'var(--radix-accordion-content-height)' },
          to: { height: '0' },
        },
        'caret-blink': {
          '0%,70%,100%': { opacity: '1' },
          '20%,50%': { opacity: '0' },
        },
      },
      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up': 'accordion-up 0.2s ease-out',
        'caret-blink': 'caret-blink 1.25s ease-out infinite',
      },
    },
  },
  plugins: [require('tailwindcss-animate'), require('@tailwindcss/line-clamp')],
};
