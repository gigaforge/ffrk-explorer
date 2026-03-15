package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const partiesDir = "data/parties"

func initPartyData() {
	userParties = make(map[int][]*SavedParty)
	partyByShare = make(map[string]int)
	os.MkdirAll(partiesDir, 0o755)
	loadAllUserParties()
}

func loadAllUserParties() {
	entries, err := os.ReadDir(partiesDir)
	if err != nil {
		return
	}
	total := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		uidStr := strings.TrimSuffix(name, ".json")
		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(partiesDir, name))
		if err != nil {
			log.Printf("WARNING: could not read %s: %v", name, err)
			continue
		}
		var parties []*SavedParty
		if err := json.Unmarshal(data, &parties); err != nil {
			log.Printf("WARNING: could not parse %s: %v", name, err)
			continue
		}
		userParties[uid] = parties
		for _, p := range parties {
			if p.ShareKey != "" {
				partyByShare[p.ShareKey] = uid
			}
		}
		total += len(parties)
	}
	if total > 0 {
		log.Printf("Loaded %d saved parties from %s/", total, partiesDir)
	}
}

func saveUserPartiesForUser(uid int) {
	userLock.RLock()
	parties := userParties[uid]
	data, err := json.Marshal(parties)
	userLock.RUnlock()

	path := filepath.Join(partiesDir, strconv.Itoa(uid)+".json")
	if len(parties) == 0 {
		os.Remove(path)
		return
	}
	if err != nil {
		log.Printf("WARNING: could not marshal parties for user %d: %v", uid, err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("WARNING: could not write %s: %v", path, err)
	}
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// computeHiddenFromVisible returns the IDs of all SBs and LMs for the given
// characters that are NOT in the visibleSet. This inverts an allowlist into a
// blocklist compatible with the existing HiddenSBs rendering logic.
func computeHiddenFromVisible(charIDs []string, visibleSet map[string]bool) []string {
	d := getAppDataSnapshot()
	if d == nil {
		return nil
	}
	var hidden []string
	for _, id := range charIDs {
		ch, ok := d.CharByID[id]
		if !ok {
			continue
		}
		for _, sb := range d.SoulBreaks[ch.Name] {
			if !visibleSet[sb.ID] {
				hidden = append(hidden, sb.ID)
			}
		}
		for _, lm := range d.LegendMateria[ch.Name] {
			if !visibleSet[lm.ID] {
				hidden = append(hidden, lm.ID)
			}
		}
	}
	sort.Strings(hidden)
	return hidden
}

// ---------- API handlers ----------

func userPartiesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userPartiesListHandler(w, r)
	case http.MethodPost:
		userPartiesSaveHandler(w, r)
	case http.MethodDelete:
		userPartiesDeleteHandler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func userPartiesListHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		jsonError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	userLock.RLock()
	parties := userParties[u.ID]
	userLock.RUnlock()
	if parties == nil {
		writeJSON(w, http.StatusOK, []*SavedParty{})
		return
	}

	writeJSON(w, http.StatusOK, parties)
}

func userPartiesSaveHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		jsonError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	var req struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		CharacterIDs []string `json:"character_ids"`
		HiddenSBs    []string `json:"hidden_sbs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(req.CharacterIDs) == 0 {
		jsonError(w, http.StatusBadRequest, "party must have at least one character")
		return
	}
	if len(req.CharacterIDs) > 5 {
		req.CharacterIDs = req.CharacterIDs[:5]
	}

	name := strings.TrimSpace(req.Name)
	if len(name) > 60 {
		name = name[:60]
	}

	// Default name to character names
	if name == "" {
		d := getAppDataSnapshot()
		if d != nil {
			var names []string
			for _, id := range req.CharacterIDs {
				if ch, ok := d.CharByID[id]; ok {
					names = append(names, ch.Name)
				}
			}
			name = strings.Join(names, ", ")
			if len(name) > 60 {
				name = name[:60]
			}
		}
	}

	var resultID string

	userLock.Lock()
	parties := userParties[u.ID]

	if req.ID != "" {
		// Update existing party
		found := false
		for _, p := range parties {
			if p.ID == req.ID {
				if p.Source == "sync" {
					userLock.Unlock()
					jsonError(w, http.StatusForbidden, "synced parties cannot be edited")
					return
				}
				p.Name = name
				p.CharacterIDs = req.CharacterIDs
				p.HiddenSBs = req.HiddenSBs
				resultID = p.ID
				found = true
				break
			}
		}
		if !found {
			userLock.Unlock()
			jsonError(w, http.StatusNotFound, "party not found")
			return
		}
	} else {
		// Create new party (synced parties don't count toward the limit)
		userCreated := 0
		for _, p := range parties {
			if p.Source != "sync" {
				userCreated++
			}
		}
		if userCreated >= 50 {
			userLock.Unlock()
			jsonError(w, http.StatusBadRequest, "maximum 50 saved parties reached")
			return
		}
		shareKey := generateID()
		for partyByShare[shareKey] != 0 {
			shareKey = generateID()
		}
		p := &SavedParty{
			ID:           generateID(),
			Name:         name,
			CharacterIDs: req.CharacterIDs,
			ShareKey:     shareKey,
			HiddenSBs:    req.HiddenSBs,
		}
		parties = append(parties, p)
		userParties[u.ID] = parties
		partyByShare[shareKey] = u.ID
		resultID = p.ID
	}
	userLock.Unlock()

	saveUserPartiesForUser(u.ID)

	// Return the saved party
	userLock.RLock()
	var saved *SavedParty
	for _, p := range userParties[u.ID] {
		if p.ID == resultID {
			saved = p
			break
		}
	}
	userLock.RUnlock()

	writeJSON(w, http.StatusOK, saved)
}

func userPartiesDeleteHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		jsonError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "id required")
		return
	}

	userLock.Lock()
	parties := userParties[u.ID]
	found := false
	for i, p := range parties {
		if p.ID == id {
			if p.Source == "sync" {
				userLock.Unlock()
				jsonError(w, http.StatusForbidden, "synced parties cannot be deleted")
				return
			}
			if p.ShareKey != "" {
				delete(partyByShare, p.ShareKey)
			}
			userParties[u.ID] = append(parties[:i], parties[i+1:]...)
			found = true
			break
		}
	}
	userLock.Unlock()

	if !found {
		jsonError(w, http.StatusNotFound, "party not found")
		return
	}

	saveUserPartiesForUser(u.ID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// ---------- shared party page handler ----------

func sharedPartyHandler(w http.ResponseWriter, r *http.Request) {
	shareKey := strings.TrimPrefix(r.URL.Path, "/party/s/")
	if shareKey == "" {
		http.NotFound(w, r)
		return
	}

	d, ok := requireAppData(w)
	if !ok {
		return
	}

	userLock.RLock()
	ownerUID, found := partyByShare[shareKey]
	if !found {
		userLock.RUnlock()
		http.NotFound(w, r)
		return
	}

	var party *SavedParty
	for _, p := range userParties[ownerUID] {
		if p.ShareKey == shareKey {
			party = p
			break
		}
	}
	ownerUser := users[ownerUID]
	userLock.RUnlock()

	if party == nil || ownerUser == nil {
		http.NotFound(w, r)
		return
	}

	var members []PartyMember
	for _, id := range party.CharacterIDs {
		ch, ok := d.CharByID[id]
		if !ok {
			continue
		}
		members = append(members, PartyMember{
			Character:     *ch,
			HeroAbilities: d.HeroAbilities[ch.Name],
			LegendMateria: d.LegendMateria[ch.Name],
			SoulBreaks:    d.SoulBreaks[ch.Name],
		})
	}

	sorted := make([]Character, len(d.Characters))
	copy(sorted, d.Characters)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	owned := snapshotOwnedSoulbreaks(ownerUID)

	hiddenSBs := make(map[string]bool)
	if party.Source == "sync" && len(party.VisibleSBs) > 0 {
		visibleSet := make(map[string]bool, len(party.VisibleSBs))
		for _, id := range party.VisibleSBs {
			visibleSet[id] = true
		}
		for _, id := range computeHiddenFromVisible(party.CharacterIDs, visibleSet) {
			hiddenSBs[id] = true
		}
	} else {
		for _, id := range party.HiddenSBs {
			hiddenSBs[id] = true
		}
	}

	data := PartyData{
		Members:         members,
		AllCharacters:   sorted,
		LoggedIn:        true,
		OwnedSoulbreaks: owned,
		IsShared:        true,
		OwnerName:       ownerUser.Username,
		PartyName:       party.Name,
		HiddenSBs:       hiddenSBs,
	}

	viewer := getCurrentUser(r)
	if viewer != nil {
		data.ViewerLoggedIn = true
		data.ViewerOwnedSoulbreaks = snapshotOwnedSoulbreaks(viewer.ID)
	}

	if len(members) > 0 {
		data.EffectSummary = buildEffectSummary(members, true, owned, hiddenSBs)
	}

	partyTmpl.Execute(w, data)
}
