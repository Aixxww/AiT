// FAQ content index. Questions and answers live in src/i18n/translations.ts
// (faq* keys); this file only orders them into categories.
//
// Restored in #75: this module was imported by src/components/faq/* but was
// never committed (the repository's `data/` ignore rule matched it), so a
// clean checkout could not build the frontend.

import type { LucideIcon } from 'lucide-react'
import {
  BookOpen,
  Bot,
  Download,
  GitPullRequest,
  LineChart,
  Settings,
  Shield,
  Sparkles,
  Wrench,
} from 'lucide-react'

export interface FAQItem {
  id: string
  questionKey: string
  answerKey: string
}

export interface FAQCategory {
  id: string
  titleKey: string
  icon: LucideIcon
  items: FAQItem[]
}

const item = (id: string, key: string): FAQItem => ({
  id,
  questionKey: `faq${key}`,
  answerKey: `faq${key}Answer`,
})

export const faqCategories: FAQCategory[] = [
  {
    id: 'getting-started',
    titleKey: 'faqCategoryGettingStarted',
    icon: BookOpen,
    items: [
      item('what-is-ait', 'WhatIsAiT'),
      item('how-does-it-work', 'HowDoesItWork'),
      item('is-profitable', 'IsProfitable'),
      item('supported-exchanges', 'SupportedExchanges'),
      item('supported-ai-models', 'SupportedAIModels'),
      item('system-requirements', 'SystemRequirements'),
    ],
  },
  {
    id: 'installation',
    titleKey: 'faqCategoryInstallation',
    icon: Download,
    items: [
      item('how-to-install', 'HowToInstall'),
      item('windows-installation', 'WindowsInstallation'),
      item('docker-deployment', 'DockerDeployment'),
      item('manual-installation', 'ManualInstallation'),
      item('server-deployment', 'ServerDeployment'),
      item('update-ait', 'UpdateAiT'),
    ],
  },
  {
    id: 'configuration',
    titleKey: 'faqCategoryConfiguration',
    icon: Settings,
    items: [
      item('configure-ai-models', 'ConfigureAIModels'),
      item('configure-exchanges', 'ConfigureExchanges'),
      item('binance-api-setup', 'BinanceAPISetup'),
      item('hyperliquid-setup', 'HyperliquidSetup'),
      item('create-strategy', 'CreateStrategy'),
      item('create-trader', 'CreateTrader'),
    ],
  },
  {
    id: 'trading',
    titleKey: 'faqCategoryTrading',
    icon: LineChart,
    items: [
      item('how-ai-decides', 'HowAIDecides'),
      item('decision-frequency', 'DecisionFrequency'),
      item('no-trades-executing', 'NoTradesExecuting'),
      item('only-short-positions', 'OnlyShortPositions'),
      item('leverage-settings', 'LeverageSettings'),
      item('stop-loss-take-profit', 'StopLossTakeProfit'),
      item('multiple-traders', 'MultipleTraders'),
      item('ai-costs', 'AICosts'),
    ],
  },
  {
    id: 'technical-issues',
    titleKey: 'faqCategoryTechnicalIssues',
    icon: Wrench,
    items: [
      item('port-in-use', 'PortInUse'),
      item('frontend-not-loading', 'FrontendNotLoading'),
      item('database-locked', 'DatabaseLocked'),
      item('talib-not-found', 'TALibNotFound'),
      item('ai-api-timeout', 'AIAPITimeout'),
      item('binance-position-mode', 'BinancePositionMode'),
      item('balance-shows-zero', 'BalanceShowsZero'),
      item('docker-pull-failed', 'DockerPullFailed'),
    ],
  },
  {
    id: 'security',
    titleKey: 'faqCategorySecurity',
    icon: Shield,
    items: [
      item('api-key-storage', 'APIKeyStorage'),
      item('encryption-details', 'EncryptionDetails'),
      item('security-best-practices', 'SecurityBestPractices'),
      item('can-ait-steal-funds', 'CanAiTStealFunds'),
    ],
  },
  {
    id: 'features',
    titleKey: 'faqCategoryFeatures',
    icon: Sparkles,
    items: [
      item('strategy-studio', 'StrategyStudio'),
      item('competition-mode', 'CompetitionMode'),
      item('chain-of-thought', 'ChainOfThought'),
    ],
  },
  {
    id: 'ai-models',
    titleKey: 'faqCategoryAIModels',
    icon: Bot,
    items: [
      item('which-ai-model-best', 'WhichAIModelBest'),
      item('custom-ai-api', 'CustomAIAPI'),
      item('ai-hallucinations', 'AIHallucinations'),
      item('compare-ai-models', 'CompareAIModels'),
    ],
  },
  {
    id: 'contributing',
    titleKey: 'faqCategoryContributing',
    icon: GitPullRequest,
    items: [
      item('how-to-contribute', 'HowToContribute'),
      item('contribute-pr-guidelines', 'PRGuidelines'),
      item('github-projects-tasks', 'BountyProgram'),
      item('report-bugs', 'ReportBugs'),
    ],
  },
]
