# FFRK Community Database

A Go web application that serves data from the Final Fantasy Record Keeper (FFRK) Community Database. Character stats, abilities, and soul breaks are loaded from CSV exports of the original Google Sheets spreadsheet and presented through a lightweight web UI.

## Features

- Character listing grouped by realm, sorted alphabetically
- Character detail pages showing:
  - Ability school proficiencies with star ratings
  - Hero Abilities unique to each character
  - Soul Breaks with tier, type, element, and effects

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
