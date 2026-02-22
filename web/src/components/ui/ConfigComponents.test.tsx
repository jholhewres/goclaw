import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigToggle,
  ConfigTextarea,
  ConfigActions,
  ConfigTagList,
  ConfigCard,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}))

describe('ConfigPage', () => {
  it('should render title and description', () => {
    render(
      <ConfigPage title="Test Title" description="Test Description">
        <div>Content</div>
      </ConfigPage>
    )

    expect(screen.getByText('Test Title')).toBeInTheDocument()
    expect(screen.getByText('Test Description')).toBeInTheDocument()
    expect(screen.getByText('Content')).toBeInTheDocument()
  })

  it('should render subtitle when provided', () => {
    render(
      <ConfigPage title="Title" subtitle="Subtitle">
        <div>Content</div>
      </ConfigPage>
    )

    expect(screen.getByText('Subtitle')).toBeInTheDocument()
  })

  it('should render success message', () => {
    render(
      <ConfigPage
        title="Title"
        message={{ type: 'success', text: 'Saved successfully!' }}
      >
        <div>Content</div>
      </ConfigPage>
    )

    expect(screen.getByText('Saved successfully!')).toBeInTheDocument()
  })

  it('should render error message', () => {
    render(
      <ConfigPage
        title="Title"
        message={{ type: 'error', text: 'An error occurred' }}
      >
        <div>Content</div>
      </ConfigPage>
    )

    expect(screen.getByText('An error occurred')).toBeInTheDocument()
  })

  it('should render actions', () => {
    render(
      <ConfigPage title="Title" actions={<button>Save</button>}>
        <div>Content</div>
      </ConfigPage>
    )

    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
  })
})

describe('ConfigSection', () => {
  it('should render title and description', () => {
    render(
      <ConfigSection title="Section Title" description="Section description">
        <div>Content</div>
      </ConfigSection>
    )

    expect(screen.getByText('Section Title')).toBeInTheDocument()
    expect(screen.getByText('Section description')).toBeInTheDocument()
  })

  it('should be collapsible when collapsible prop is true', async () => {
    render(
      <ConfigSection title="Collapsible Section" collapsible defaultCollapsed>
        <div>Hidden Content</div>
      </ConfigSection>
    )

    // Initially collapsed (because defaultCollapsed is true), content should not be visible
    expect(screen.queryByText('Hidden Content')).not.toBeInTheDocument()

    // Click to expand
    await userEvent.click(screen.getByText('Collapsible Section'))

    // Now content should be visible
    expect(screen.getByText('Hidden Content')).toBeInTheDocument()
  })

  it('should start expanded when collapsible is true but defaultCollapsed is false', () => {
    render(
      <ConfigSection title="Collapsible Section" collapsible defaultCollapsed={false}>
        <div>Visible Content</div>
      </ConfigSection>
    )

    // Content should be visible by default
    expect(screen.getByText('Visible Content')).toBeInTheDocument()
  })

  it('should start collapsed when defaultCollapsed is true', () => {
    render(
      <ConfigSection title="Section" collapsible defaultCollapsed>
        <div>Content</div>
      </ConfigSection>
    )

    expect(screen.queryByText('Content')).not.toBeInTheDocument()
  })
})

describe('ConfigField', () => {
  it('should render label', () => {
    render(
      <ConfigField label="Field Label">
        <input />
      </ConfigField>
    )

    expect(screen.getByText('Field Label')).toBeInTheDocument()
  })

  it('should render hint when provided', () => {
    render(
      <ConfigField label="Label" hint="This is a hint">
        <input />
      </ConfigField>
    )

    expect(screen.getByText('This is a hint')).toBeInTheDocument()
  })
})

describe('ConfigInput', () => {
  it('should render input with value', () => {
    render(
      <ConfigInput value="test value" onChange={() => {}} />
    )

    expect(screen.getByDisplayValue('test value')).toBeInTheDocument()
  })

  it('should call onChange when value changes', async () => {
    const handleChange = vi.fn()
    render(
      <ConfigInput value="" onChange={handleChange} />
    )

    await userEvent.type(screen.getByRole('textbox'), 'a')

    expect(handleChange).toHaveBeenCalledWith('a')
  })

  it('should render placeholder', () => {
    render(
      <ConfigInput value="" onChange={() => {}} placeholder="Enter value" />
    )

    expect(screen.getByPlaceholderText('Enter value')).toBeInTheDocument()
  })

  it('should be disabled when disabled prop is true', () => {
    render(
      <ConfigInput value="" onChange={() => {}} disabled />
    )

    expect(screen.getByRole('textbox')).toBeDisabled()
  })
})

describe('ConfigSelect', () => {
  const options = [
    { value: 'option1', label: 'Option 1' },
    { value: 'option2', label: 'Option 2' },
  ]

  it('should render select with options', () => {
    render(
      <ConfigSelect value="option1" onChange={() => {}} options={options} />
    )

    expect(screen.getByDisplayValue('Option 1')).toBeInTheDocument()
  })

  it('should call onChange when selection changes', async () => {
    const handleChange = vi.fn()
    render(
      <ConfigSelect value="option1" onChange={handleChange} options={options} />
    )

    await userEvent.selectOptions(screen.getByRole('combobox'), 'option2')

    expect(handleChange).toHaveBeenCalledWith('option2')
  })
})

describe('ConfigToggle', () => {
  it('should render toggle with label', () => {
    render(
      <ConfigToggle enabled={false} onChange={() => {}} label="Enable feature" />
    )

    expect(screen.getByText('Enable feature')).toBeInTheDocument()
  })

  it('should call onChange when clicked', async () => {
    const handleChange = vi.fn()
    render(
      <ConfigToggle enabled={false} onChange={handleChange} label="Toggle" />
    )

    await userEvent.click(screen.getByText('Toggle'))

    expect(handleChange).toHaveBeenCalledWith(true)
  })

  it('should be disabled when disabled prop is true', async () => {
    const handleChange = vi.fn()
    render(
      <ConfigToggle enabled={false} onChange={handleChange} label="Toggle" disabled />
    )

    await userEvent.click(screen.getByText('Toggle'))

    expect(handleChange).not.toHaveBeenCalled()
  })
})

describe('ConfigTextarea', () => {
  it('should render textarea with value', () => {
    render(
      <ConfigTextarea value="test content" onChange={() => {}} />
    )

    expect(screen.getByDisplayValue('test content')).toBeInTheDocument()
  })

  it('should call onChange when value changes', async () => {
    const handleChange = vi.fn()
    render(
      <ConfigTextarea value="" onChange={handleChange} />
    )

    await userEvent.type(screen.getByRole('textbox'), 'a')

    expect(handleChange).toHaveBeenCalledWith('a')
  })
})

describe('ConfigActions', () => {
  it('should render save button', () => {
    render(
      <ConfigActions onSave={() => {}} />
    )

    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
  })

  it('should render reset button when hasChanges is true', () => {
    render(
      <ConfigActions onSave={() => {}} onReset={() => {}} hasChanges />
    )

    expect(screen.getByRole('button', { name: 'Reset' })).toBeInTheDocument()
  })

  it('should not render reset button when hasChanges is false', () => {
    render(
      <ConfigActions onSave={() => {}} onReset={() => {}} hasChanges={false} />
    )

    expect(screen.queryByRole('button', { name: 'Reset' })).not.toBeInTheDocument()
  })

  it('should disable save button when hasChanges is false', () => {
    render(
      <ConfigActions onSave={() => {}} hasChanges={false} />
    )

    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  })

  it('should show saving state', () => {
    render(
      <ConfigActions onSave={() => {}} saving />
    )

    expect(screen.getByText('Saving...')).toBeInTheDocument()
  })

  it('should call onSave when save button is clicked', async () => {
    const handleSave = vi.fn()
    render(
      <ConfigActions onSave={handleSave} hasChanges />
    )

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(handleSave).toHaveBeenCalled()
  })

  it('should call onReset when reset button is clicked', async () => {
    const handleReset = vi.fn()
    render(
      <ConfigActions onSave={() => {}} onReset={handleReset} hasChanges />
    )

    await userEvent.click(screen.getByRole('button', { name: 'Reset' }))

    expect(handleReset).toHaveBeenCalled()
  })
})

describe('ConfigTagList', () => {
  it('should render tags', () => {
    render(
      <ConfigTagList tags={['tag1', 'tag2']} onAdd={() => {}} onRemove={() => {}} />
    )

    expect(screen.getByText('tag1')).toBeInTheDocument()
    expect(screen.getByText('tag2')).toBeInTheDocument()
  })

  it('should call onRemove when remove button is clicked', async () => {
    const handleRemove = vi.fn()
    render(
      <ConfigTagList tags={['tag1']} onAdd={() => {}} onRemove={handleRemove} />
    )

    // Find the remove button (X icon) within the tag
    const removeButton = screen.getByRole('button', { name: '' })
    await userEvent.click(removeButton)

    expect(handleRemove).toHaveBeenCalledWith('tag1')
  })

  it('should call onAdd when adding a new tag', async () => {
    const handleAdd = vi.fn()
    render(
      <ConfigTagList tags={[]} onAdd={handleAdd} onRemove={() => {}} />
    )

    const input = screen.getByPlaceholderText('Add item...')
    await userEvent.type(input, 'newtag{Enter}')

    expect(handleAdd).toHaveBeenCalledWith('newtag')
  })

  it('should show empty message when no tags and not readOnly', () => {
    render(
      <ConfigTagList tags={[]} onAdd={() => {}} onRemove={() => {}} readOnly={false} />
    )

    expect(screen.getByText('No items')).toBeInTheDocument()
  })

  it('should not show empty message when readOnly', () => {
    render(
      <ConfigTagList tags={[]} onAdd={() => {}} onRemove={() => {}} readOnly />
    )

    expect(screen.queryByText('No items')).not.toBeInTheDocument()
  })
})

describe('ConfigCard', () => {
  it('should render title and subtitle', () => {
    render(
      <ConfigCard title="Card Title" subtitle="Card subtitle">
        <div>Content</div>
      </ConfigCard>
    )

    expect(screen.getByText('Card Title')).toBeInTheDocument()
    expect(screen.getByText('Card subtitle')).toBeInTheDocument()
  })

  it('should render actions', () => {
    render(
      <ConfigCard title="Title" actions={<button>Action</button>}>
        <div>Content</div>
      </ConfigCard>
    )

    expect(screen.getByRole('button', { name: 'Action' })).toBeInTheDocument()
  })

  it('should render children', () => {
    render(
      <ConfigCard title="Title">
        <div>Card Content</div>
      </ConfigCard>
    )

    expect(screen.getByText('Card Content')).toBeInTheDocument()
  })
})

describe('LoadingSpinner', () => {
  it('should render loading spinner', () => {
    const { container } = render(<LoadingSpinner />)

    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })
})

describe('ErrorState', () => {
  it('should render error message', () => {
    render(<ErrorState message="Something went wrong" />)

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
  })

  it('should render retry button when onRetry is provided', () => {
    render(<ErrorState onRetry={() => {}} />)

    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('should call onRetry when retry button is clicked', async () => {
    const handleRetry = vi.fn()
    render(<ErrorState onRetry={handleRetry} />)

    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(handleRetry).toHaveBeenCalled()
  })
})
