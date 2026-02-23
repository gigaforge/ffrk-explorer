# FFRK Community Database

A Go web application that serves data from the Final Fantasy Record Keeper (FFRK) Community Database. Character stats, abilities, and soul breaks are loaded from CSV exports of the original Google Sheets spreadsheet and presented through a lightweight web UI.

## Features

### Character Browsing
- Character listing grouped by realm, sorted alphabetically
- Character detail pages showing:
  - Ability school proficiencies with star ratings
  - Hero Abilities unique to each character
  - Soul Breaks with tier, type, element, and effects

### Search & Filtering
- Search bar with filtering by character name, realm, SB tier, element, and imperil
- Additional Effects modal with filters for break counters, damage boosts (Weakness, Magical, PHY, Sorcery, Pentabreak), Deshell, and Deprotect
- Job Requirements filter modal for school-based character filtering
- SB Tier dropdown dynamically populated from CSV data
- All search matching is case-insensitive
- Collapsible per-character results with Collapse All / Expand All buttons
- Up to 500 results displayed with truncation warning

### Soul Break Details
- Expandable status effect details on soul breaks with duration and description
- Burst commands (BSB) with expandable status effects
- Brave commands with level breakdown for Brave Mode soul breaks
- Synchro abilities (SASB) with expandable details
- Zenith abilities (ZSB) with expandable details
- Dual Shift details (DASB) with expandable status effects
- Arcane Dyad finisher details (ADSB)

### User Accounts
- User authentication with login/registration
- Per-user soulbreak ownership tracking with checkbox toggles on search results

### Image Caching
- Character, hero ability, and soul break images cached locally and served from disk
- Parallelized image caching with 20 goroutine workers

### Data Management
- Auto-updates CSVs from Google Sheets every 6 hours
- External sync endpoint (`/ffrk_sync.php`) for soulbreak sync tools

## Running

```sh
go build -o ffrk-server main.go
./ffrk-server
```

The server starts on port 9090 by default. Open http://localhost:9090 in your browser.

## Data

CSV files are exported from the [FFRK Community Database](https://docs.google.com/spreadsheets/d/1f8OJIQhpycljDQ8QNDk_va1GJ1u7RVoMaNjFcHH0LKk) Google Sheet. The app currently uses:

- `Characters.csv` — character stats, weapon/armor proficiency, and ability school ratings
- `Soul-Breaks.csv` — all soul breaks linked by character name
- `Hero-Abilities.csv` — character-specific hero abilities
