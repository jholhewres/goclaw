import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ApiConfig } from '@/pages/ApiConfig'
import * as apiModule from '@/lib/api'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'apiConfig.title': 'API Configuration',
        'apiConfig.subtitle': 'Settings',
        'apiConfig.description': 'Configure your LLM provider',
        'apiConfig.providerSection': 'Provider',
        'apiConfig.providerSectionDesc': 'Select your LLM provider',
        'apiConfig.connectionSection': 'Connection',
        'apiConfig.connectionSectionDesc': 'API endpoint and authentication',
        'apiConfig.baseUrl': 'Base URL',
        'apiConfig.baseUrlHint': 'Leave empty to use default',
        'apiConfig.apiKey': 'API Key',
        'apiConfig.apiKeyPlaceholder': 'Enter your API key',
        'apiConfig.apiKeyHint': 'Encrypted and stored in vault',
        'apiConfig.model': 'Model',
        'apiConfig.modelHint': 'Enter the model name',
        'apiConfig.selectModel': 'Select a model',
        'apiConfig.testConnection': 'Test Connection',
        'apiConfig.testing': 'Testing...',
        'apiConfig.connectionFailed': 'Connection failed',
        'apiConfig.statusTitle': 'Connection Status',
        'apiConfig.statusConfigured': 'Configured and ready',
        'apiConfig.statusNotConfigured': 'API key not configured',
        'apiConfig.currentProvider': 'Current Provider',
        'common.save': 'Save',
        'common.saving': 'Saving...',
        'common.reset': 'Reset',
        'common.success': 'Success',
        'common.error': 'Error',
      }
      return translations[key] || key
    },
  }),
}))

// Mock API
vi.mock('@/lib/api', () => ({
  api: {
    config: {
      get: vi.fn(),
      update: vi.fn(),
    },
    setup: {
      testProvider: vi.fn(),
    },
  },
}))

const mockConfig = {
  provider: 'openai',
  base_url: 'https://api.openai.com/v1',
  api_key_configured: true,
  api_key_masked: '••••••••6789',
  model: 'gpt-4o-mini',
  models: ['gpt-4o-mini', 'gpt-4o', 'gpt-3.5-turbo'],
}

describe('ApiConfig', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(apiModule.api.config.get).mockResolvedValue(mockConfig as unknown as Record<string, unknown>)
    vi.mocked(apiModule.api.config.update).mockResolvedValue(mockConfig as unknown as Record<string, unknown>)
    vi.mocked(apiModule.api.setup.testProvider).mockResolvedValue({ success: true })
  })

  it('should render loading state initially', () => {
    render(<ApiConfig />)

    // Loading spinner should be visible while loading
    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('should render provider cards after loading', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })

    // Check other providers are shown
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
    expect(screen.getByText('Google AI')).toBeInTheDocument()
  })

  it('should select provider when clicked', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })

    // Click on Anthropic provider
    await userEvent.click(screen.getByText('Anthropic'))

    // Check that the provider card shows as selected (has the blue check icon)
    const anthropicCard = screen.getByText('Anthropic').closest('button')
    expect(anthropicCard).toHaveClass('border-[#3b82f6]')
  })

  it('should test connection when test button is clicked', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('Test Connection')).toBeInTheDocument()
    })

    await userEvent.click(screen.getByText('Test Connection'))

    await waitFor(() => {
      expect(apiModule.api.setup.testProvider).toHaveBeenCalled()
    })
  })

  it('should save configuration when save button is clicked', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
    })

    // Make a change to enable the save button
    await userEvent.click(screen.getByText('Anthropic'))

    // Now the save button should be enabled
    const saveButton = screen.getByRole('button', { name: 'Save' })
    await userEvent.click(saveButton)

    await waitFor(() => {
      expect(apiModule.api.config.update).toHaveBeenCalled()
    })
  })

  it('should show status card with configured status', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('Connection Status')).toBeInTheDocument()
    })

    expect(screen.getByText('Configured and ready')).toBeInTheDocument()
    expect(screen.getByText('openai')).toBeInTheDocument()
  })

  it('should allow changing base URL', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('Base URL')).toBeInTheDocument()
    })

    const baseUrlInput = document.querySelector('input[value="https://api.openai.com/v1"]')
    expect(baseUrlInput).toBeInTheDocument()
  })

  it('should allow entering new API key', async () => {
    render(<ApiConfig />)

    await waitFor(() => {
      expect(screen.getByText('API Key')).toBeInTheDocument()
    })

    // Check that password input exists (type password)
    const passwordInput = document.querySelector('input[type="password"]')
    expect(passwordInput).toBeInTheDocument()
  })
})
