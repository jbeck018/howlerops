import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { InsightBriefPanel } from '../reports/insight-brief-panel'
import type { InsightBriefResponse } from '@/lib/insight-api'

const fullBrief: InsightBriefResponse = {
  brief: 'Revenue grew steadily over the period.',
  rowCount: 42,
  forecastMethod: 'holt',
  predictions: [
    { time: '2026-02-01T00:00:00Z', value: 120, lower: 110, upper: 130 },
    { time: '2026-02-02T00:00:00Z', value: 125, lower: 113, upper: 137 },
  ],
  anomalies: [{ time: '2026-01-15T00:00:00Z', value: 9999, expected: 120, score: 8.3 }],
}

describe('InsightBriefPanel', () => {
  it('shows a loading state', () => {
    render(<InsightBriefPanel brief={null} loading />)
    expect(screen.getByTestId('insight-loading')).toBeInTheDocument()
  })

  it('shows an error state', () => {
    render(<InsightBriefPanel brief={null} error="provider unavailable" />)
    expect(screen.getByText('provider unavailable')).toBeInTheDocument()
  })

  it('renders the narrative, forecast method, anomalies, and row count', () => {
    render(<InsightBriefPanel brief={fullBrief} />)
    expect(screen.getByText('Revenue grew steadily over the period.')).toBeInTheDocument()
    expect(screen.getByText('holt')).toBeInTheDocument()
    expect(screen.getByText(/Anomalies \(1\)/)).toBeInTheDocument()
    expect(screen.getByText(/2026-01-15/)).toBeInTheDocument()
    expect(screen.getByText(/Based on 42 rows/)).toBeInTheDocument()
  })

  it('surfaces a non-fatal forecast error', () => {
    render(
      <InsightBriefPanel
        brief={{ brief: 'No trend.', rowCount: 5, forecastError: 'no time column' }}
      />,
    )
    expect(screen.getByText(/Forecast skipped: no time column/)).toBeInTheDocument()
  })

  it('shows an empty prompt when there is no brief', () => {
    render(<InsightBriefPanel brief={null} />)
    expect(screen.getByText(/Run a query and generate a brief/)).toBeInTheDocument()
  })
})
