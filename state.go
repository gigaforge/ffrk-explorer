package main

import "sync"

// ---------- globals ----------

var (
	characters       []Character
	soulBreaks       map[string][]SoulBreak      // keyed by character name
	heroAbilities    map[string][]HeroAbility    // keyed by character name
	burstCommands    map[string][]BurstCommand   // keyed by "Character|Source"
	synchroAbilities map[string][]SynchroAbility // keyed by "Character|Source"
	zenithAbilities  map[string][]ZenithAbility  // keyed by "Character|Source"
	braveCommands    map[string]*BraveCommand    // keyed by "Character|Source"
	statusEffects    map[string]StatusEffect     // keyed by Common Name
	realmGroups      []RealmGroup
	charByID         map[string]*Character
	tierNames        []string
	dataLock         sync.RWMutex
)

// ---------- user globals ----------

var (
	users          map[int]*User           // id → user
	usersByName    map[string]*User        // lowercase username → user
	usersByAPI     map[string]*User        // api_key → user
	userSoulbreaks map[int]map[string]bool // user_id → set of soulbreak IDs
	sessions       map[string]int          // session_token → user_id
	userLock       sync.RWMutex
	nextUserID     int
)

// ---------- CSV auto-update ----------

var csvSheets = []csvSheet{
	{"Characters.csv", "1771023676", []string{"Realm", "Name", "ID"}},
	{"Soul-Breaks.csv", "344457459", []string{"Character", "Name", "Tier"}},
	{"Hero-Abilities.csv", "329671300", []string{"Character", "Name", "Type"}},
	{"Burst.csv", "1373487754", []string{"Character", "Source", "Name"}},
	{"Synchro.csv", "13552509", []string{"Character", "Source", "Name"}},
	{"Zenith-SB-Abilities.csv", "1801274757", []string{"Character", "Source", "Name"}},
	{"Brave.csv", "1286318057", []string{"Character", "Source", "Name"}},
	{"Status.csv", "1899148923", []string{"Common Name", "Effects"}},
}

const sheetID = "1f8OJIQhpycljDQ8QNDk_va1GJ1u7RVoMaNjFcHH0LKk"
