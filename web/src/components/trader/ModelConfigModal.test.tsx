// @vitest-environment jsdom
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ModelConfigModal } from './ModelConfigModal'
import type { AIModel } from '../../types'

// #75: Settings → AI models → "Add other model". Placeholder values only.

const supported: AIModel[] = [
  { id: 'deepseek', name: 'DeepSeek', provider: 'deepseek', enabled: false },
  {
    id: 'custom',
    name: 'Other (OpenAI-compatible)',
    provider: 'custom',
    enabled: false,
  },
]

function renderModal(
  onSave = vi.fn(),
  editingModelId: string | null = null,
  configured: AIModel[] = []
) {
  render(
    <ModelConfigModal
      allModels={supported}
      configuredModels={configured}
      editingModelId={editingModelId}
      onSave={onSave}
      onDelete={vi.fn()}
      onClose={vi.fn()}
      language="en"
    />
  )
  return onSave
}

describe('ModelConfigModal custom (other) model entry', () => {
  it('offers an "Add other model" entry and saves name, URL and model', () => {
    const onSave = renderModal()
    fireEvent.click(screen.getByTestId('add-other-model'))

    fireEvent.change(screen.getByPlaceholderText('e.g. WorkBuddy DeepSeek'), {
      target: { value: 'WorkBuddy' },
    })
    const inputs = document.querySelectorAll('input')
    const key = Array.from(inputs).find((i) => i.type === 'password')!
    const url = Array.from(inputs).find((i) => i.type === 'url')!
    const model = Array.from(inputs)
      .filter((i) => i.type === 'text')
      .pop()!
    fireEvent.change(key, { target: { value: 'sk-placeholder' } })
    fireEvent.change(url, {
      target: { value: 'https://gateway.example.com/v2' },
    })
    fireEvent.change(model, { target: { value: 'deepseek-v4.1-flash' } })
    expect(url.required).toBe(true)
    expect(model.required).toBe(true)

    fireEvent.submit(key.closest('form')!)
    expect(onSave).toHaveBeenCalledWith(
      'custom',
      'sk-placeholder',
      'https://gateway.example.com/v2',
      'deepseek-v4.1-flash',
      'WorkBuddy'
    )
  })

  it('does not save a custom entry without a model name', () => {
    const onSave = renderModal()
    fireEvent.click(screen.getByTestId('add-other-model'))
    fireEvent.change(screen.getByPlaceholderText('e.g. WorkBuddy DeepSeek'), {
      target: { value: 'WorkBuddy' },
    })
    const inputs = Array.from(document.querySelectorAll('input'))
    fireEvent.change(inputs.find((i) => i.type === 'password')!, {
      target: { value: 'sk-placeholder' },
    })
    fireEvent.change(inputs.find((i) => i.type === 'url')!, {
      target: { value: 'https://gateway.example.com/v2' },
    })
    fireEvent.submit(inputs[0].closest('form')!)
    expect(onSave).not.toHaveBeenCalled()
  })

  it('edits an existing custom entry with its saved name', () => {
    const onSave = renderModal(vi.fn(), 'u1_custom_ab12cd34', [
      {
        id: 'u1_custom_ab12cd34',
        name: 'WorkBuddy',
        provider: 'custom',
        enabled: true,
        has_api_key: true,
        customApiUrl: 'https://gateway.example.com/v2',
        customModelName: 'deepseek-v4.1-flash',
      },
    ])
    const name = screen.getByPlaceholderText(
      'e.g. WorkBuddy DeepSeek'
    ) as HTMLInputElement
    expect(name.value).toBe('WorkBuddy')
    fireEvent.change(name, { target: { value: 'WorkBuddy v2' } })
    const key = Array.from(document.querySelectorAll('input')).find(
      (i) => i.type === 'password'
    )!
    fireEvent.change(key, { target: { value: 'sk-placeholder-2' } })
    fireEvent.submit(key.closest('form')!)
    expect(onSave).toHaveBeenCalledWith(
      'u1_custom_ab12cd34',
      'sk-placeholder-2',
      'https://gateway.example.com/v2',
      'deepseek-v4.1-flash',
      'WorkBuddy v2'
    )
  })
})
