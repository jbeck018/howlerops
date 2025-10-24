# Authentication System - Visual Guide

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Frontend (React)                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐         ┌──────────────┐                    │
│  │   Header     │         │  Auth Store  │                    │
│  │              │◄────────┤   (Zustand)  │                    │
│  │ [AuthButton] │         │              │                    │
│  └──────────────┘         └──────┬───────┘                    │
│         │                         │                            │
│         │ click                   │                            │
│         ▼                         │                            │
│  ┌──────────────┐                │                            │
│  │  AuthModal   │                │                            │
│  │              │                │                            │
│  │ ┌──────────┐ │                │                            │
│  │ │  Login   │─┼────────────────┘                            │
│  │ │  Signup  │ │                                             │
│  │ └──────────┘ │                                             │
│  └──────────────┘                                             │
│                                                                 │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTP/HTTPS
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Backend (Go)                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  /api/auth/signup    - Create new user account                 │
│  /api/auth/login     - Authenticate & issue JWT                │
│  /api/auth/refresh   - Refresh access token                    │
│  /api/auth/logout    - Invalidate refresh token                │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## User Flow Diagrams

### Sign Up Flow

```
User → Click "Sign In" → AuthModal Opens
                              │
                              ▼
                         Click "Sign Up" Tab
                              │
                              ▼
                         Fill Form:
                         ┌─────────────────┐
                         │ Username        │
                         │ Email           │
                         │ Password        │
                         │   ✓ 8+ chars    │
                         │   ✓ Uppercase   │
                         │   ✓ Number      │
                         │ Confirm Pass    │
                         └─────────────────┘
                              │
                              ▼
                         Click "Sign Up"
                              │
                              ▼
                     POST /api/auth/signup
                              │
                    ┌─────────┴─────────┐
                    │                   │
                 Success             Error
                    │                   │
                    ▼                   ▼
            Store user & tokens    Show error alert
                    │
                    ▼
            Modal closes
                    │
                    ▼
            Username in header
```

### Sign In Flow

```
User → Click "Sign In" → AuthModal Opens
                              │
                              ▼
                         "Login" Tab Active
                              │
                              ▼
                         Fill Form:
                         ┌─────────────────┐
                         │ Username/Email  │
                         │ Password        │
                         └─────────────────┘
                              │
                              ▼
                         Click "Sign In"
                              │
                              ▼
                     POST /api/auth/login
                              │
                    ┌─────────┴─────────┐
                    │                   │
                 Success             Error
                    │                   │
                    ▼                   ▼
            Store user & tokens    Show error alert
                    │
                    ▼
            Start refresh timer
                    │
                    ▼
            Check tier/license
                    │
                    ▼
            Modal closes
                    │
                    ▼
            Username in header
```

### Token Refresh Flow

```
App Running
    │
    ├─────► Timer: 5 min before token expires
    │           │
    │           ▼
    │       POST /api/auth/refresh
    │           │
    │      ┌────┴────┐
    │      │         │
    │   Success    Fail
    │      │         │
    │      ▼         ▼
    │   Update    Sign Out
    │   tokens    User
    │      │
    │      ▼
    │   Schedule
    │   next refresh
    │      │
    └──────┘
```

### Sign Out Flow

```
User → Click username → Dropdown Menu
                              │
                              ▼
                         Click "Sign Out"
                              │
                              ▼
                     POST /api/auth/logout
                              │
                              ▼
                         Clear tokens
                              │
                              ▼
                         Stop refresh timer
                              │
                              ▼
                         Reset tier to local
                              │
                              ▼
                         Show "Sign In" button
```

## UI Components Layout

### Header (Unauthenticated)

```
┌────────────────────────────────────────────────────────────────┐
│ 🐺 HowlerOps  Dashboard  Connections      [Sign In]  🛡️ Local │
└────────────────────────────────────────────────────────────────┘
                                                  │
                                                  └─► Opens AuthModal
```

### Header (Authenticated)

```
┌────────────────────────────────────────────────────────────────┐
│ 🐺 HowlerOps  Dashboard  Connections  [👤 testuser]  🛡️ Pro   │
└────────────────────────────────────────────────────────────────┘
                                              │
                                              └─► User Menu:
                                                  ┌──────────────┐
                                                  │ testuser     │
                                                  │ test@ex.com  │
                                                  ├──────────────┤
                                                  │ Settings     │
                                                  │ Subscription │
                                                  ├──────────────┤
                                                  │ Sign Out     │
                                                  └──────────────┘
```

### AuthModal - Login Tab

```
┌─────────────────────────────────────┐
│  Welcome to SQL Studio              │
│  Sign in to sync your data...       │
├─────────────────────────────────────┤
│                                     │
│  ┌────────┬────────┐               │
│  │ Login  │ SignUp │               │
│  └────────┴────────┘               │
│                                     │
│  Username or Email                  │
│  ┌─────────────────────────────┐   │
│  │                             │   │
│  └─────────────────────────────┘   │
│                                     │
│  Password                           │
│  ┌─────────────────────────────┐   │
│  │ ••••••••••                  │   │
│  └─────────────────────────────┘   │
│                                     │
│  ┌───────────────────────────┐     │
│  │      Sign In              │     │
│  └───────────────────────────┘     │
│                                     │
└─────────────────────────────────────┘
```

### AuthModal - Signup Tab

```
┌─────────────────────────────────────┐
│  Welcome to SQL Studio              │
│  Sign in to sync your data...       │
├─────────────────────────────────────┤
│                                     │
│  ┌────────┬────────┐               │
│  │ Login  │ SignUp │               │
│  └────────┴────────┘               │
│                                     │
│  Username                           │
│  ┌─────────────────────────────┐   │
│  │                             │   │
│  └─────────────────────────────┘   │
│                                     │
│  Email                              │
│  ┌─────────────────────────────┐   │
│  │                             │   │
│  └─────────────────────────────┘   │
│                                     │
│  Password                           │
│  ┌─────────────────────────────┐   │
│  │ ••••••••••                  │   │
│  └─────────────────────────────┘   │
│                                     │
│  ✓ At least 8 characters            │
│  ✓ One uppercase letter             │
│  ✓ One number                       │
│                                     │
│  Confirm Password                   │
│  ┌─────────────────────────────┐   │
│  │ ••••••••••                  │   │
│  └─────────────────────────────┘   │
│                                     │
│  ┌───────────────────────────┐     │
│  │      Sign Up              │     │
│  └───────────────────────────┘     │
│                                     │
└─────────────────────────────────────┘
```

### Error State

```
┌─────────────────────────────────────┐
│  Welcome to SQL Studio              │
├─────────────────────────────────────┤
│  ┌────────┬────────┐               │
│  │ Login  │ SignUp │               │
│  └────────┴────────┘               │
│                                     │
│  ⚠ Invalid credentials              │
│                                     │
│  Username or Email                  │
│  ┌─────────────────────────────┐   │
│  │ testuser                    │   │
│  └─────────────────────────────┘   │
│                                     │
│  Password                           │
│  ┌─────────────────────────────┐   │
│  │ ••••••••••                  │   │
│  └─────────────────────────────┘   │
│                                     │
│  ┌───────────────────────────┐     │
│  │      Sign In              │     │
│  └───────────────────────────┘     │
└─────────────────────────────────────┘
```

### Loading State

```
┌─────────────────────────────────────┐
│  Welcome to SQL Studio              │
├─────────────────────────────────────┤
│  ┌────────┬────────┐               │
│  │ Login  │ SignUp │               │
│  └────────┴────────┘               │
│                                     │
│  Username or Email                  │
│  ┌─────────────────────────────┐   │
│  │ testuser              (disabled) │
│  └─────────────────────────────┘   │
│                                     │
│  Password                           │
│  ┌─────────────────────────────┐   │
│  │ ••••••••••           (disabled) │
│  └─────────────────────────────┘   │
│                                     │
│  ┌───────────────────────────┐     │
│  │ ⟳ Signing in...          │     │
│  └───────────────────────────┘     │
└─────────────────────────────────────┘
```

## State Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                      Auth Store State                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Initial State:                                              │
│  ┌────────────────────────────────────────────────────┐     │
│  │ user: null                                         │     │
│  │ tokens: null                                       │     │
│  │ isAuthenticated: false                             │     │
│  │ isLoading: false                                   │     │
│  │ error: null                                        │     │
│  └────────────────────────────────────────────────────┘     │
│                          │                                   │
│                          │ signIn() / signUp()               │
│                          ▼                                   │
│  Loading State:                                              │
│  ┌────────────────────────────────────────────────────┐     │
│  │ isLoading: true                                    │     │
│  │ error: null                                        │     │
│  └────────────────────────────────────────────────────┘     │
│                          │                                   │
│                    ┌─────┴─────┐                            │
│                    │           │                            │
│                 Success     Error                           │
│                    │           │                            │
│                    ▼           ▼                            │
│  Authenticated:              Error State:                   │
│  ┌──────────────────┐       ┌──────────────────┐           │
│  │ user: {...}      │       │ error: "message" │           │
│  │ tokens: {...}    │       │ isLoading: false │           │
│  │ isAuth: true     │       └──────────────────┘           │
│  │ isLoading: false │                                       │
│  └──────────────────┘                                       │
│         │                                                   │
│         │ refreshToken() [auto]                             │
│         │                                                   │
│         ├──► Success: Update tokens                         │
│         └──► Failure: signOut()                             │
│                                                              │
│  signOut()                                                   │
│         │                                                   │
│         ▼                                                   │
│  Back to Initial State                                       │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## Data Flow

```
┌─────────────┐        ┌─────────────┐        ┌─────────────┐
│   User      │        │  Component  │        │ Auth Store  │
│   Action    │───────▶│   (Form)    │───────▶│  (Zustand)  │
└─────────────┘        └─────────────┘        └──────┬──────┘
                                                      │
                                                      │ API Call
                                                      ▼
                                              ┌───────────────┐
                                              │  Go Backend   │
                                              └───────┬───────┘
                                                      │
                                                      │ Response
                                                      ▼
                                              ┌───────────────┐
                                              │   Update      │
                                              │   Store       │
                                              └───────┬───────┘
                                                      │
                                                      │ Notify
                                                      ▼
                                              ┌───────────────┐
                                              │  Re-render    │
                                              │  Components   │
                                              └───────────────┘
```

## Token Lifecycle

```
Sign In/Sign Up
    │
    ▼
┌─────────────────────────────────────┐
│ Access Token (1 hour)               │
│ Refresh Token (7 days)              │
└───────────────┬─────────────────────┘
                │
                │ Stored in localStorage
                │ (via Zustand persist)
                │
                ▼
┌─────────────────────────────────────┐
│ App Restart                         │
│ ↓                                   │
│ Load from localStorage              │
│ ↓                                   │
│ Tokens still valid ✓                │
└───────────────┬─────────────────────┘
                │
                │ 55 min later...
                │
                ▼
┌─────────────────────────────────────┐
│ Auto Refresh Timer Triggers         │
│ (5 min before expiration)           │
│ ↓                                   │
│ POST /api/auth/refresh              │
│ ↓                                   │
│ New Access Token (1 hour)           │
│ New Refresh Token (7 days)          │
└───────────────┬─────────────────────┘
                │
                │ Repeat...
                │
                │ If refresh fails:
                │
                ▼
┌─────────────────────────────────────┐
│ Auto Sign Out                       │
│ ↓                                   │
│ Clear tokens                        │
│ ↓                                   │
│ Show "Sign In" button               │
└─────────────────────────────────────┘
```

## Integration Points

```
┌────────────────────────────────────────────────────────────┐
│                     Application                             │
├────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐         ┌──────────────┐                │
│  │    Header    │◄────────┤  Auth Store  │                │
│  │ [AuthButton] │         └──────┬───────┘                │
│  └──────────────┘                │                         │
│                                   │                         │
│  ┌──────────────┐                │                         │
│  │  Tier Store  │◄───────────────┘                         │
│  │  Integration │                                          │
│  └──────────────┘                                          │
│       │                                                    │
│       │ On Sign In: Check license                          │
│       │ On Sign Out: Reset to local                        │
│       ▼                                                    │
│  ┌──────────────┐                                          │
│  │ Tier Badge   │                                          │
│  └──────────────┘                                          │
│                                                             │
│  ┌──────────────┐                                          │
│  │ Protected    │                                          │
│  │ Routes       │                                          │
│  └──────────────┘                                          │
│       │                                                    │
│       │ Redirect if not authenticated                      │
│       ▼                                                    │
│  [Feature Pages]                                           │
│                                                             │
└────────────────────────────────────────────────────────────┘
```

## Summary

This visual guide shows:
- ✅ Complete system architecture
- ✅ User flow for all auth actions
- ✅ UI component layouts
- ✅ State management flow
- ✅ Token lifecycle
- ✅ Integration with tier system

All using **shadcn/ui components exclusively**!
