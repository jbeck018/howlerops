# Query Templates - Visual Component Guide

## Component Overview

This guide describes the visual design and layout of all template components.

---

## 1. Templates Page (`TemplatesPage.tsx`)

### Layout Structure

```
┌─────────────────────────────────────────────────────────────────┐
│ Header Section (bg-card, border-b)                              │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ 📄 Query Templates                    [+ New Template]      │ │
│ │ Browse, execute, and schedule reusable query templates      │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                  │
│ Search & Filters                                                │
│ ┌────────────────────────┬──────────────┬──────────────────┐   │
│ │ 🔍 Search templates... │ [Most Used ▼]│ [Clear Filters]  │   │
│ └────────────────────────┴──────────────┴──────────────────┘   │
│                                                                  │
│ Category Tabs                                                   │
│ [All (127)] [Reporting (45)] [Analytics (32)] [Maintenance...] │
│                                                                  │
│ Tag Filters                                                     │
│ 🏷️ Tags: [sales] [reporting] [daily] [analytics] +12 more      │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ Template Grid (3 columns on desktop, 1 on mobile)               │
│                                                                  │
│ ┌──────────┐ ┌──────────┐ ┌──────────┐                        │
│ │ Template │ │ Template │ │ Template │                        │
│ │  Card 1  │ │  Card 2  │ │  Card 3  │                        │
│ └──────────┘ └──────────┘ └──────────┘                        │
│                                                                  │
│ ┌──────────┐ ┌──────────┐ ┌──────────┐                        │
│ │ Template │ │ Template │ │ Template │                        │
│ │  Card 4  │ │  Card 5  │ │  Card 6  │                        │
│ └──────────┘ └──────────┘ └──────────┘                        │
└─────────────────────────────────────────────────────────────────┘
```

### Color Scheme
- **Background**: `bg-background` (light gray in light mode, dark in dark mode)
- **Header**: `bg-card` with `border-b`
- **Primary Action**: Blue button
- **Search**: Muted input with search icon

---

## 2. Template Card (`TemplateCard.tsx`)

### Card Layout

```
┌──────────────────────────────────────────────────────┐
│ Daily Sales Report                              ⋮    │
│ Generate comprehensive sales report for date         │
│ range with status filtering                          │
│                                                       │
│ [Reporting] [🔓 Public]                              │
│ ─────────────────────────────────────────────────── │
│ [sales] [reporting] [daily] +2 more                  │
│                                                       │
│ ┌───────────────────────────────────────────────┐   │
│ │ SELECT                                         │   │
│ │   DATE(order_date) as date,                   │   │
│ │   COUNT(*) as total_orders...                 │   │
│ └───────────────────────────────────────────────┘   │
│ 3 parameters                                         │
│ ─────────────────────────────────────────────────── │
│ 📈 145 uses          Updated 3 days ago             │
│                                                       │
│ [▶ Use Template]                                     │
└──────────────────────────────────────────────────────┘
```

### Visual Elements
- **Category Badge**: Color-coded by category
  - Reporting: Blue (`bg-blue-100 text-blue-700`)
  - Analytics: Purple (`bg-purple-100 text-purple-700`)
  - Maintenance: Orange (`bg-orange-100 text-orange-700`)
  - Custom: Gray (`bg-gray-100 text-gray-700`)

- **Public/Private Badge**: Lock/Users icon
- **SQL Preview**: Monospace font, muted background, 3 lines max
- **Hover State**: Shadow lift, actions menu appears
- **Actions Menu** (⋮): Execute, Schedule, Duplicate, Edit, Delete

---

## 3. Template Executor (`TemplateExecutor.tsx`)

### Modal Layout

```
┌────────────────────────────────────────────────────────────┐
│ ▶ Execute: Daily Sales Report                         [×] │
│ Generate comprehensive sales report for date range         │
├────────────────────────────────────────────────────────────┤
│ [Parameters (3)] [SQL Preview] [Results]                   │
├────────────────────────────────────────────────────────────┤
│                                                             │
│ Start Date *                                    [date]     │
│ Filter orders from this date                               │
│ ┌────────────────────┐                                     │
│ │ 2024-01-01         │                                     │
│ └────────────────────┘                                     │
│                                                             │
│ End Date *                                      [date]     │
│ Filter orders up to this date                              │
│ ┌────────────────────┐                                     │
│ │ 2024-12-31         │                                     │
│ └────────────────────┘                                     │
│                                                             │
│ Status                                         [string]    │
│ Order status to filter                                     │
│ ┌────────────────────┐                                     │
│ │ completed      ▼   │                                     │
│ └────────────────────┘                                     │
│                                                             │
├────────────────────────────────────────────────────────────┤
│ ✓ Query executed successfully in 234ms. 365 rows returned │
├────────────────────────────────────────────────────────────┤
│                                   [Reset] [▶ Execute Query]│
└────────────────────────────────────────────────────────────┘
```

### Parameter Input Types
- **String**: Text input
- **Number**: Number input with min/max
- **Boolean**: Checkbox
- **Date**: Date picker
- **Enum (options)**: Dropdown select

### SQL Preview Tab
```
┌────────────────────────────────────────────────────────────┐
│ SELECT                                                      │
│   DATE(order_date) as date,                                │
│   COUNT(*) as total_orders,                                │
│   SUM(total_amount) as revenue                             │
│ FROM orders                                                 │
│ WHERE order_date BETWEEN '2024-01-01' AND '2024-12-31'    │
│   AND status = 'completed'                                 │
│ GROUP BY DATE(order_date)                                  │
└────────────────────────────────────────────────────────────┘
```
- Syntax highlighting (SQL)
- Parameters interpolated in preview
- Read-only view

### Results Tab
```
┌────────────────────────────────────────────────────────────┐
│ date       │ total_orders │ revenue   │ avg_order_value   │
├────────────┼──────────────┼───────────┼───────────────────┤
│ 2024-01-23 │ 156          │ 12450.00  │ 79.81            │
│ 2024-01-22 │ 189          │ 15230.50  │ 80.58            │
│ 2024-01-21 │ 142          │ 11890.25  │ 83.73            │
└────────────────────────────────────────────────────────────┘
```

---

## 4. Cron Builder (`CronBuilder.tsx`)

### Tabs Layout

```
┌────────────────────────────────────────────────────────────┐
│ [Presets] [Custom] [Advanced]                              │
├────────────────────────────────────────────────────────────┤
│ PRESETS TAB                                                 │
│                                                             │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Every Hour                                             │ │
│ │ Runs at the start of every hour                        │ │
│ └────────────────────────────────────────────────────────┘ │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Daily at 9 AM                                          │ │
│ │ Runs once per day at 9:00 AM                           │ │
│ └────────────────────────────────────────────────────────┘ │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Weekly on Monday                                       │ │
│ │ Runs every Monday at 9:00 AM                           │ │
│ └────────────────────────────────────────────────────────┘ │
├────────────────────────────────────────────────────────────┤
│ 🕐 Runs once per day at 9:00 AM                            │
│ Expression: 0 9 * * *                                      │
└────────────────────────────────────────────────────────────┘
```

### Custom Builder Tab

```
┌────────────────────────────────────────────────────────────┐
│ [Presets] [Custom] [Advanced]                              │
├────────────────────────────────────────────────────────────┤
│ Hour                    Minute                              │
│ ┌──────────────┐       ┌──────────────┐                    │
│ │ 09:00    ▼   │       │ :00      ▼   │                    │
│ └──────────────┘       └──────────────┘                    │
│                                                             │
│ Day of Week                                                 │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Weekdays (Mon-Fri)                                 ▼   │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                             │
│ Day of Month                                                │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Every day                                          ▼   │ │
│ └────────────────────────────────────────────────────────┘ │
├────────────────────────────────────────────────────────────┤
│ 🕐 Runs Monday through Friday at 9:00 AM                   │
│ Expression: 0 9 * * 1-5                                    │
└────────────────────────────────────────────────────────────┘
```

### Advanced Tab

```
┌────────────────────────────────────────────────────────────┐
│ [Presets] [Custom] [Advanced]                              │
├────────────────────────────────────────────────────────────┤
│ Cron Expression                                             │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ 0 9 * * *                                              │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                             │
│ ℹ️ Format: minute hour day month weekday                   │
│   Use * for any value, */n for every n units              │
├────────────────────────────────────────────────────────────┤
│ 🕐 Runs once per day at 9:00 AM                            │
│ Expression: 0 9 * * *                                      │
└────────────────────────────────────────────────────────────┘
```

---

## 5. Schedule Creator (`ScheduleCreator.tsx`)

### Modal Layout

```
┌────────────────────────────────────────────────────────────┐
│ 📅 Schedule Query                                      [×] │
│ Create scheduled query to run automatically                │
├────────────────────────────────────────────────────────────┤
│                                                             │
│ Template *                                                  │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Daily Sales Report                                 ▼   │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                             │
│ Schedule Name *                                             │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Daily sales report                                     │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                             │
│ Schedule *                                                  │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ [CRON BUILDER COMPONENT]                               │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                             │
│ SQL Query                                                   │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ SELECT DATE(order_date)...                             │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                             │
│ Parameters                                                  │
│ [Parameter inputs based on template]                       │
│                                                             │
│ Notification Email (Optional)                               │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ team@company.com                                       │ │
│ └────────────────────────────────────────────────────────┘ │
│ Receive email notifications when query completes or fails  │
│                                                             │
│ Result Storage                                              │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Store in Database                                  ▼   │ │
│ └────────────────────────────────────────────────────────┘ │
│ Choose where to store query results                        │
│                                                             │
│ ℹ️ The scheduled query will run automatically according    │
│   to the specified frequency                               │
├────────────────────────────────────────────────────────────┤
│                                 [Cancel] [Create Schedule] │
└────────────────────────────────────────────────────────────┘
```

---

## 6. Schedules Page (`SchedulesPage.tsx`)

### Layout Structure

```
┌─────────────────────────────────────────────────────────────┐
│ Header Section                                               │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 📅 Scheduled Queries                  [+ New Schedule]  │ │
│ │ Manage automated query execution schedules              │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                              │
│ Statistics Cards                                             │
│ ┌─────────┬─────────┬─────────┬─────────┐                  │
│ │ Total   │ Active  │ Paused  │ Failed  │                  │
│ │   4     │   3     │   1     │   0     │                  │
│ └─────────┴─────────┴─────────┴─────────┘                  │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ Schedules List                                               │
│                                                              │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Daily Sales Report - Morning         [✓ Active]     ⋮  │ │
│ │ Template: Daily Sales Report                            │ │
│ │─────────────────────────────────────────────────────────│ │
│ │ Schedule: Daily at 9:00 AM                              │ │
│ │ Next Run: in 2 hours    Last Run: 1 hour ago           │ │
│ │ Notifications: reports@company.com                      │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                              │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Weekly User Activity Analysis        [✓ Active]     ⋮  │ │
│ │ Template: User Activity Analysis                        │ │
│ │─────────────────────────────────────────────────────────│ │
│ │ Schedule: Every Monday at 9:00 AM                       │ │
│ │ Next Run: in 5 days     Last Run: 1 day ago            │ │
│ │ Notifications: analytics@company.com                    │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Schedule Card Actions Menu
```
⋮
├─ ▶ Run Now
├─ ⏸️ Pause / ▶ Resume
├─ 📊 View History
├─ ────────────
└─ 🗑️ Delete
```

---

## 7. Execution History (`ExecutionHistory.tsx`)

### Modal Layout

```
┌────────────────────────────────────────────────────────────┐
│ 📊 Execution History                                   [×] │
│ Daily Sales Report - Morning - Last 50 executions          │
├────────────────────────────────────────────────────────────┤
│ Statistics Cards                                            │
│ ┌────────┬────────┬────────┬────────┐                      │
│ │ Total  │Success │  Avg   │ Total  │                      │
│ │ Runs   │ Rate   │Duration│ Rows   │                      │
│ │  50    │ 96.0%  │ 234ms  │18,250  │                      │
│ └────────┴────────┴────────┴────────┘                      │
│                                                             │
│ Status Breakdown                                            │
│ 🟢 48 Successful  🔴 2 Failed  🟠 0 Timeout                │
│                                                             │
│ Timeline                                                    │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ [✓ Success]              2 hours ago                   │ │
│ │ Time: Jan 23, 09:00  Duration: 234ms  Rows: 365       │ │
│ └────────────────────────────────────────────────────────┘ │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ [✓ Success]              1 day ago                     │ │
│ │ Time: Jan 22, 09:00  Duration: 198ms  Rows: 365       │ │
│ └────────────────────────────────────────────────────────┘ │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ [✗ Failed]               2 days ago                    │ │
│ │ Time: Jan 21, 09:00  Duration: 5421ms                 │ │
│ │ Query timeout: execution exceeded 5000ms limit         │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                             │
│ Execution Details (when item selected)                     │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Execution ID: exec-1                                   │ │
│ │ Executed At: January 23, 2024 at 9:00 AM              │ │
│ │ Duration: 234ms                                        │ │
│ │ Rows Returned: 365                                     │ │
│ │                                                        │ │
│ │ Result Preview:                                        │ │
│ │ ┌─────────┬─────────┬──────────┬──────────┐          │ │
│ │ │ date    │ orders  │ revenue  │ avg_val  │          │ │
│ │ ├─────────┼─────────┼──────────┼──────────┤          │ │
│ │ │01-23    │ 156     │ 12450.00 │ 79.81    │          │ │
│ │ │01-22    │ 189     │ 15230.50 │ 80.58    │          │ │
│ │ └─────────┴─────────┴──────────┴──────────┘          │ │
│ └────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

---

## Responsive Design

### Desktop (≥1024px)
- Template cards: 3 columns grid
- Full sidebar and navigation
- Expanded modal dialogs (max-w-5xl)
- Side-by-side parameter inputs

### Tablet (768px - 1023px)
- Template cards: 2 columns grid
- Collapsible sidebar
- Medium modal dialogs (max-w-3xl)
- Stacked parameter inputs

### Mobile (<768px)
- Template cards: 1 column stack
- Bottom sheet navigation
- Full-screen modals
- Vertical parameter layout
- Simplified statistics (2 columns)

---

## Dark Mode Support

All components support dark mode via Tailwind's dark: prefix:

- **Background**: `dark:bg-gray-900`
- **Cards**: `dark:bg-gray-800`
- **Text**: `dark:text-gray-100`
- **Borders**: `dark:border-gray-700`
- **Badges**: Dark variants for each category
- **Syntax Highlighting**: `vscDarkPlus` theme

---

## Accessibility Features

### Keyboard Navigation
- Tab through all interactive elements
- Enter/Space to activate buttons
- Escape to close modals
- Arrow keys in dropdowns

### Screen Readers
- ARIA labels on all inputs
- Role attributes on custom components
- Live regions for dynamic content
- Skip links for navigation

### Focus Management
- Visible focus indicators
- Focus trap in modals
- Focus return after modal close
- Logical tab order

---

## Animation & Transitions

### Hover Effects
- Card lift: `hover:shadow-lg transition-all duration-200`
- Button highlight: `hover:bg-primary/90`
- Tag selection: Scale up slightly

### Loading States
- Skeleton loaders with pulse animation
- Spinner for actions: `animate-spin`
- Progress bars for long operations

### Modal Transitions
- Fade in background overlay
- Scale up content (0.95 → 1.0)
- Slide up on mobile

---

## Color Palette

### Status Colors
- Success: `green-500` / `green-100` background
- Error: `red-500` / `red-100` background
- Warning: `yellow-500` / `yellow-100` background
- Info: `blue-500` / `blue-100` background

### Category Colors
- Reporting: Blue tones
- Analytics: Purple tones
- Maintenance: Orange tones
- Custom: Gray tones

### Semantic Colors
- Primary: Brand blue
- Secondary: Muted gray
- Accent: Highlight color
- Destructive: Red for delete actions

---

This visual guide provides a complete reference for the design system and component layouts used throughout the templates feature.
