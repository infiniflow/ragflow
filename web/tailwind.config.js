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
        '2xl': '1536px',
      },
    },
    screens: {
      sm: '640px',
      md: '768px',
      lg: '1024px',
      xl: '1280px',
      '2xl': '1536px',
      '3xl': '1780px',
      '4xl': '1980px',
    },
    extend: {
      borderWidth: {
        0.5: '0.5px',
      },
      colors: {
        border: 'var(--border-default)',
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
        'colors-text-neutral-weak': 'var(--colors-text-neutral-weak)',
        'colors-text-functional-danger': 'var(--colors-text-functional-danger)',
        'colors-text-inverse-strong': 'var(--colors-text-inverse-strong)',
        'colors-text-persist-light': 'var(--colors-text-persist-light)',
        'colors-text-inverse-weak': 'var(--colors-text-inverse-weak)',

        'background-badge': 'var(--background-badge)',
        'text-badge': 'var(--text-badge)',
        'text-title': 'var(--text-title)',
        'text-sub-title': 'var(--text-sub-title)',
        'text-sub-title-invert': 'var(--text-sub-title-invert)',
        'text-title-invert': 'var(--text-title-invert)',
        'background-header-bar': 'var(--background-header-bar)',
        'background-card': 'var(--background-card)',
        'background-note': 'var(--background-note)',
        'background-highlight': 'var(--background-highlight)',

        'input-border': 'var(--input-border)',

        /* design colors */
        'bg-title': 'var(--bg-title)',
        'bg-base': 'var(--bg-base)',
        'bg-card': 'var(--bg-card)',
        'bg-component': 'var(--bg-component)',
        'bg-input': 'var(--bg-input)',
        'bg-canvas': {
          DEFAULT: 'rgb(var(--bg-canvas) / <alpha-value>)',
        },
        'bg-list': {
          DEFAULT: 'rgb(var(--bg-list) / <alpha-value>)',
        },
        'text-primary': {
          DEFAULT: 'rgb(var(--text-primary) / <alpha-value>)',
        },
        'text-primary-inverse': {
          DEFAULT: 'rgb(var(--text-primary-inverse) / <alpha-value>)',
        },
        'text-secondary': {
          DEFAULT: 'rgb(var(--text-secondary) / <alpha-value>)',
        },
        'text-secondary-inverse': {
          DEFAULT: 'rgb(var(--text-secondary-inverse) / <alpha-value>)',
        },
        'text-disabled': 'var(--text-disabled)',
        'text-input-tip': 'var(--text-input-tip)',
        'border-default': 'var(--border-default)',
        'border-accent': 'var(--border-accent)',
        'border-button': 'var(--border-button)',
        'accent-primary': {
          DEFAULT: 'rgb(var(--accent-primary) / <alpha-value>)',
          5: 'rgba(var(--accent-primary) / 0.05)', // 5%
        },
        'bg-accent': 'var(--bg-accent)',
        'state-success': {
          DEFAULT: 'rgb(var(--state-success) / <alpha-value>)',
          5: 'rgba(var(--state-success) / 0.05)', // 5%
        },
        'state-warning': {
          DEFAULT: 'rgb(var(--state-warning) / <alpha-value>)',
          5: 'rgba(var(--state-warning) / 0.05)', // 5%
        },
        'state-error': {
          DEFAULT: 'rgb(var(--state-error) / <alpha-value>)',
          5: 'rgba(var(--state-error) / 0.05)', // 5%
        },
        'team-group': 'var(--team-group)',
        'team-member': 'var(--team-member)',
        'team-department': 'var(--team-department)',
        'bg-group': 'var(--bg-group)',
        'bg-member': 'var(--bg-member)',
        'bg-department': 'var(--bg-department)',

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
        backgroundCoreWeak: {
          DEFAULT: 'var(--background-core-weak)',
          foreground: 'var(--background-core-weak-foreground)',
        },
        'colors-background-inverse-standard': {
          DEFAULT: 'var(--colors-background-inverse-standard)',
          foreground: 'var(--colors-background-inverse-standard-foreground)',
        },
        'colors-background-inverse-standard': {
          DEFAULT: 'var(--colors-background-inverse-standard)',
          foreground: 'var(--background-inverse-standard-foreground)',
        },
        'colors-background-inverse-strong': {
          DEFAULT: 'var(--colors-background-inverse-strong)',
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
        sidebar: {
          DEFAULT: 'hsl(var(--sidebar-background))',
          foreground: 'hsl(var(--sidebar-foreground))',
          primary: 'hsl(var(--sidebar-primary))',
          'primary-foreground': 'hsl(var(--sidebar-primary-foreground))',
          accent: 'hsl(var(--sidebar-accent))',
          'accent-foreground': 'hsl(var(--sidebar-accent-foreground))',
          border: 'hsl(var(--sidebar-border))',
          ring: 'hsl(var(--sidebar-ring))',
        },
      },
      backgroundImage: {
        'metallic-gradient':
          'linear-gradient(104deg, rgb(var(--text-primary)) 30%, var(--metallic) 50%, rgb(var(--text-primary)) 70%)',
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
  plugins: [
    require('tailwindcss-animate'),
    require('@tailwindcss/line-clamp'),
    require('tailwind-scrollbar'),
  ],
};
