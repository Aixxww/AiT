import js from '@eslint/js'
import tseslint from '@typescript-eslint/eslint-plugin'
import tsparser from '@typescript-eslint/parser'
import react from 'eslint-plugin-react'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import prettier from 'eslint-plugin-prettier'

export default [
  {
    ignores: ['dist', 'node_modules', 'build', '*.config.js']
  },
  js.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      parser: tsparser,
      parserOptions: {
        ecmaVersion: 'latest',
        sourceType: 'module',
        ecmaFeatures: {
          jsx: true
        }
      },
      globals: {
        window: 'readonly',
        document: 'readonly',
        console: 'readonly',
        setTimeout: 'readonly',
        clearTimeout: 'readonly',
        setInterval: 'readonly',
        clearInterval: 'readonly',
        fetch: 'readonly',
        localStorage: 'readonly',
        sessionStorage: 'readonly'
      }
    },
    plugins: {
      '@typescript-eslint': tseslint,
      'react': react,
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
      'prettier': prettier
    },
    rules: {
      ...tseslint.configs.recommended.rules,
      ...react.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,

      // Prettier integration
      'prettier/prettier': 'error',

      // React rules
      'react/react-in-jsx-scope': 'off',
      'react/prop-types': 'off',
      // 该规则在 TS 项目中经常与 TS 的类型检查重复，关闭以避免误报
      'no-undef': 'off',

      // TypeScript rules
      // 放宽以下规则以避免在不改变功能的情况下大面积改动代码
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/explicit-module-boundary-types': 'off',
      '@typescript-eslint/no-unused-vars': 'off',

      // React Refresh
      'react-refresh/only-export-components': 'off',

      // General rules
      'no-console': 'off',
      'no-debugger': 'off',

      // 新版 react-hooks 推荐规则在本项目会造成大量误报，关闭以免影响开发体验
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/static-components': 'off',
      'react-hooks/preserve-manual-memoization': 'off',

      // 某些字符串中包含未转义字符用于展示，关闭以避免不必要的修改
      'react/no-unescaped-entities': 'off',

      // 可视情况关闭依赖数组校验（如需严格可改为 'warn'）
      'react-hooks/exhaustive-deps': 'off',

      // ------------------------------------------------------------------
      // Design-token guard (frontend-design-overhaul-proposal-20260727 §P2.5)
      // Blocks new hardcoded hex colors, raw Tailwind palette classes and
      // legacy ait-* alias classes so the P0 token cleanup cannot regress.
      // Whitelisted palette/branding files are exempted in the override
      // block below; one-off third-party brand colors use inline disables.
      // ------------------------------------------------------------------
      'no-restricted-syntax': [
        'error',
        {
          selector:
            'Literal[value=/#(?!(?:[fF]{6}|0{6})\\b)[0-9a-fA-F]{6}\\b/]',
          message:
            'Hardcoded hex color. Use a semantic token (var(--color-*) or a semantic Tailwind class). Palette data files are whitelisted in eslint.config.js.'
        },
        {
          selector: 'Literal[value=/#[0-9a-fA-F]{8}\\b/]',
          message:
            'Hardcoded hex color with alpha. Use a semantic token, e.g. color-mix(in srgb, var(--color-*) N%, transparent).'
        },
        {
          selector: 'Literal[value=/^#(?![fF]{3}$|0{3}$)[0-9a-fA-F]{3,4}$/]',
          message:
            'Hardcoded short hex color. Use a semantic token (var(--color-*)).'
        },
        {
          selector:
            'TemplateElement[value.raw=/#(?!(?:[fF]{6}|0{6})\\b)[0-9a-fA-F]{6}\\b/]',
          message:
            'Hardcoded hex color in template literal. Use a semantic token (var(--color-*)).'
        },
        {
          selector: 'TemplateElement[value.raw=/#[0-9a-fA-F]{8}\\b/]',
          message:
            'Hardcoded hex color with alpha in template literal. Use a semantic token.'
        },
        {
          selector:
            'Literal[value=/(?:text|bg|border|ring|from|to|via|shadow|fill|stroke|divide|outline|decoration|accent|caret|placeholder)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-[0-9]{2,3}(?![0-9])/]',
          message:
            'Raw Tailwind palette class. Use the semantic scale instead (profit/loss/warning/info/primary/muted/surface/panel/...).'
        },
        {
          selector:
            'TemplateElement[value.raw=/(?:text|bg|border|ring|from|to|via|shadow|fill|stroke|divide|outline|decoration|accent|caret|placeholder)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-[0-9]{2,3}(?![0-9])/]',
          message:
            'Raw Tailwind palette class in template literal. Use the semantic scale instead.'
        },
        {
          selector:
            'Literal[value=/\\bait-(?:gold|bg|accent|text|success|danger)/]',
          message:
            'Legacy ait-* alias class. The compatibility aliases were removed in P0.3a; use the semantic token class directly.'
        },
        {
          selector:
            'TemplateElement[value.raw=/\\bait-(?:gold|bg|accent|text|success|danger)/]',
          message:
            'Legacy ait-* alias class in template literal. Use the semantic token class directly.'
        }
      ]
    },
    settings: {
      react: {
        version: 'detect'
      }
    }
  },
  {
    // Design-token guard whitelist (proposal §P0.3 口径): these files carry
    // programmatic palettes or brand/marketing colors that are data, not UI
    // theme styling. The landing page keeps its fixed dark mockup styling.
    files: [
      'src/utils/chartTheme.ts',
      'src/utils/traderColors.ts',
      'src/components/common/PunkAvatar.tsx',
      'src/components/common/ModelIcons.tsx',
      'src/components/common/ExchangeIcons.tsx',
      'src/components/charts/AdvancedChart.tsx',
      'src/pages/SettingsPage.tsx',
      'src/components/landing/**/*'
    ],
    rules: {
      'no-restricted-syntax': 'off'
    }
  }
]
