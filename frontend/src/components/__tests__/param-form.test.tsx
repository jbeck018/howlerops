import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { ParamInput } from '@/lib/param-types'

import { ParamForm } from '../shared/param-form'

describe('ParamForm', () => {
  it('shows a message when there are no inputs', () => {
    render(<ParamForm inputs={[]} values={{}} onChange={() => {}} />)
    expect(screen.getByText(/no parameters/i)).toBeInTheDocument()
  })

  it('renders a labelled text input and reports changes', () => {
    const onChange = vi.fn()
    const inputs: ParamInput[] = [{ name: 'status', type: 'string', label: 'Status', required: true }]
    render(<ParamForm inputs={inputs} values={{}} onChange={onChange} />)

    // Required marker + label.
    expect(screen.getByText('Status')).toBeInTheDocument()
    const input = screen.getByLabelText(/Status/i)
    fireEvent.change(input, { target: { value: 'active' } })
    expect(onChange).toHaveBeenCalledWith({ status: 'active' })
  })

  it('coerces number inputs to numbers', () => {
    const onChange = vi.fn()
    const inputs: ParamInput[] = [{ name: 'limit', type: 'integer' }]
    render(<ParamForm inputs={inputs} values={{}} onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('limit'), { target: { value: '10' } })
    expect(onChange).toHaveBeenCalledWith({ limit: 10 })
  })

  it('clears a number input to undefined when emptied (not NaN)', () => {
    const onChange = vi.fn()
    const inputs: ParamInput[] = [{ name: 'limit', type: 'number' }]
    render(<ParamForm inputs={inputs} values={{ limit: 5 }} onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('limit'), { target: { value: '' } })
    expect(onChange).toHaveBeenCalledWith({ limit: undefined })
  })

  it('does not store NaN for an empty number value', () => {
    const onChange = vi.fn()
    const inputs: ParamInput[] = [{ name: 'limit', type: 'number' }]
    // A NaN incoming value must render as an empty controlled input, not "NaN".
    render(<ParamForm inputs={inputs} values={{ limit: NaN }} onChange={onChange} />)
    expect(screen.getByLabelText('limit')).toHaveValue(null)
  })

  it('parses a list input into an array', () => {
    const onChange = vi.fn()
    const inputs: ParamInput[] = [{ name: 'ids', type: 'list' }]
    render(<ParamForm inputs={inputs} values={{}} onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('ids'), { target: { value: 'a, b ,c' } })
    expect(onChange).toHaveBeenCalledWith({ ids: ['a', 'b', 'c'] })
  })
})
