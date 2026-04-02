Read `CLAUDE.md` and execute the following onboarding protocol in full.

## Onboarding Protocol

You are starting a new session on ModaMi Mobile. Execute these steps in order before doing anything else:

### 1. Read project instructions
Read `CLAUDE.md` completely — understand the tech stack, navigation flow, state management, design tokens, available agents, critical rules, and changelog convention.

### 2. Read developer guide
Read `docs/DEVELOPMENT.md` — understand the component library, styling patterns, store patterns, and troubleshooting tips.

### 3. Check current backlog
Read `TODO.md` if it exists — understand what is currently in progress, up next, and in the backlog.

### 4. Check recent changelogs
Scan `docs/changelogs/` — read any recently updated changelog files to understand what has changed recently.

### 5. Summarise and ask
Present a brief summary to the user:
- Current tech stack and state (mock data vs. real backend)
- What is currently in progress (from TODO.md)
- Any pending work or blockers
- What you are ready to help with

Then ask: "What would you like to work on?"

## Quick Reference

**Run the app**:
```bash
npm start                    # Metro bundler
npm run ios                  # iOS simulator
npm run android              # Android emulator
```

**After config changes**: `npm start -- --reset-cache`

**Check types**: `npx tsc --noEmit`

**Lint**: `npm run lint`

**Demo credentials**: `demo@modami.app` / `demo1234`
