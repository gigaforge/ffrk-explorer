package main

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---------- user data persistence ----------

const soulbreaksDir = "data/soulbreaks"

func initUserData() {
	users = make(map[int]*User)
	usersByName = make(map[string]*User)
	usersByAPI = make(map[string]*User)
	userSoulbreaks = make(map[int]map[string]bool)
	sessions = make(map[string]int)
	nextUserID = 1

	usersJSON := "data/users.json"

	os.MkdirAll("data", 0o755)
	os.MkdirAll(soulbreaksDir, 0o755)

	if _, err := os.Stat(usersJSON); err == nil {
		loadUsersJSON(usersJSON)
		loadAllUserSoulbreaks()
	} else {
		log.Println("No user data found, starting fresh")
	}
	loadSessions()
}

func loadUsersJSON(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("WARNING: could not read %s: %v", path, err)
		return
	}
	var userList []User
	if err := json.Unmarshal(data, &userList); err != nil {
		log.Printf("WARNING: could not parse %s: %v", path, err)
		return
	}
	for i := range userList {
		u := &userList[i]
		users[u.ID] = u
		usersByName[strings.ToLower(u.Username)] = u
		if u.APIKey != "" {
			usersByAPI[u.APIKey] = u
		}
		if u.ID >= nextUserID {
			nextUserID = u.ID + 1
		}
	}
	log.Printf("Loaded %d users from %s", len(users), path)
}

func loadAllUserSoulbreaks() {
	entries, err := os.ReadDir(soulbreaksDir)
	if err != nil {
		log.Printf("WARNING: could not read %s: %v", soulbreaksDir, err)
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
		data, err := os.ReadFile(filepath.Join(soulbreaksDir, name))
		if err != nil {
			log.Printf("WARNING: could not read %s: %v", name, err)
			continue
		}
		var ids []string
		if err := json.Unmarshal(data, &ids); err != nil {
			log.Printf("WARNING: could not parse %s: %v", name, err)
			continue
		}
		set := make(map[string]bool, len(ids))
		for _, id := range ids {
			set[id] = true
		}
		userSoulbreaks[uid] = set
		total += len(ids)
	}
	log.Printf("Loaded %d user soulbreak records from %s/", total, soulbreaksDir)
}

const sessionsFile = "data/sessions.json"

func loadSessions() {
	data, err := os.ReadFile(sessionsFile)
	if err != nil {
		return
	}
	var raw map[string]int
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("WARNING: could not parse %s: %v", sessionsFile, err)
		return
	}
	loaded := 0
	for token, uid := range raw {
		if _, ok := users[uid]; ok {
			sessions[token] = uid
			loaded++
		}
	}
	log.Printf("Loaded %d sessions from %s", loaded, sessionsFile)
}

func saveSessions() {
	data, err := json.Marshal(sessions)
	if err != nil {
		log.Printf("WARNING: could not marshal sessions: %v", err)
		return
	}
	if err := os.WriteFile(sessionsFile, data, 0o644); err != nil {
		log.Printf("WARNING: could not write %s: %v", sessionsFile, err)
	}
}

func saveUsersJSON(path string) {
	userList := snapshotUsers()
	data, err := json.Marshal(userList)
	if err != nil {
		log.Printf("WARNING: could not marshal users: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("WARNING: could not write %s: %v", path, err)
	}
}

func saveUserSoulbreaksForUser(uid int) {
	path := filepath.Join(soulbreaksDir, strconv.Itoa(uid)+".json")
	ids := snapshotUserSoulbreakIDs(uid)
	if len(ids) == 0 {
		os.Remove(path)
		return
	}
	data, err := json.Marshal(ids)
	if err != nil {
		log.Printf("WARNING: could not marshal soulbreaks for user %d: %v", uid, err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("WARNING: could not write %s: %v", path, err)
	}
}

func saveAllUserSoulbreaks() {
	userLock.RLock()
	uids := make([]int, 0, len(userSoulbreaks))
	for uid := range userSoulbreaks {
		uids = append(uids, uid)
	}
	userLock.RUnlock()
	for _, uid := range uids {
		saveUserSoulbreaksForUser(uid)
	}
}

func snapshotUsers() []User {
	userLock.RLock()
	defer userLock.RUnlock()

	userList := make([]User, 0, len(users))
	for _, u := range users {
		userList = append(userList, *u)
	}
	sort.Slice(userList, func(i, j int) bool { return userList[i].ID < userList[j].ID })
	return userList
}

func snapshotUserSoulbreakIDs(uid int) []string {
	userLock.RLock()
	defer userLock.RUnlock()

	set := userSoulbreaks[uid]
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func snapshotOwnedSoulbreaks(uid int) map[string]bool {
	userLock.RLock()
	defer userLock.RUnlock()

	src := userSoulbreaks[uid]
	if src == nil {
		return make(map[string]bool)
	}
	dst := make(map[string]bool, len(src))
	for id, owned := range src {
		dst[id] = owned
	}
	return dst
}

// ---------- session management ----------

func generateSessionToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand failed: %v", err)
	}
	return hex.EncodeToString(b)
}

func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password + "ffrk"))
	return hex.EncodeToString(h[:])
}

func createSession(w http.ResponseWriter, userID int) {
	token := generateSessionToken()
	userLock.Lock()
	sessions[token] = userID
	saveSessions()
	userLock.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func getCurrentUser(r *http.Request) *User {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	userLock.RLock()
	defer userLock.RUnlock()
	uid, ok := sessions[cookie.Value]
	if !ok {
		return nil
	}
	return users[uid]
}

// ---------- auth API handlers ----------

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" || req.Password == "" {
		jsonError(w, http.StatusUnauthorized, "Username and password are required")
		return
	}

	userLock.RLock()
	u := usersByName[strings.ToLower(req.Username)]
	userLock.RUnlock()
	if u == nil {
		jsonError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	sha256Hex := hashPassword(req.Password)
	// PHP uses $2y$ prefix; Go's bcrypt accepts $2a$ — replace prefix for compat
	storedHash := strings.Replace(u.PasswordHash, "$2y$", "$2a$", 1)
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(sha256Hex)); err != nil {
		jsonError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	createSession(w, u.ID)
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "api_key": u.APIKey})
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" || len(req.Password) < 6 {
		jsonError(w, http.StatusBadRequest, "Username required, password must be at least 6 characters")
		return
	}
	if len(req.Username) > 50 {
		jsonError(w, http.StatusBadRequest, "Username too long (max 50 characters)")
		return
	}

	userLock.Lock()
	if _, exists := usersByName[strings.ToLower(req.Username)]; exists {
		userLock.Unlock()
		jsonError(w, http.StatusConflict, "Username already taken")
		return
	}

	sha256Hex := hashPassword(req.Password)
	hash, err := bcrypt.GenerateFromPassword([]byte(sha256Hex), bcrypt.DefaultCost)
	if err != nil {
		userLock.Unlock()
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	apiKey := fmt.Sprintf("%x", md5.Sum([]byte(time.Now().String()+req.Username)))
	u := &User{
		ID:           nextUserID,
		Username:     req.Username,
		PasswordHash: string(hash),
		APIKey:       apiKey,
	}
	nextUserID++
	users[u.ID] = u
	usersByName[strings.ToLower(u.Username)] = u
	usersByAPI[u.APIKey] = u
	userLock.Unlock()

	saveUsersJSON("data/users.json")
	createSession(w, u.ID)
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "api_key": u.APIKey})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	cookie, err := r.Cookie("session")
	if err == nil {
		userLock.Lock()
		delete(sessions, cookie.Value)
		saveSessions()
		userLock.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		jsonError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "api_key": u.APIKey})
}

func userSoulbreaksGetHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		jsonError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	userLock.RLock()
	owned := userSoulbreaks[u.ID]
	var ids []string
	for id := range owned {
		ids = append(ids, id)
	}
	userLock.RUnlock()
	sort.Strings(ids)
	writeJSON(w, http.StatusOK, ids)
}

func userSoulbreaksPostHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		jsonError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	var req struct {
		SoulbreakID string `json:"soulbreak_id"`
		Owned       bool   `json:"owned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.SoulbreakID == "" {
		http.Error(w, "soulbreak_id required", http.StatusBadRequest)
		return
	}

	userLock.Lock()
	if userSoulbreaks[u.ID] == nil {
		userSoulbreaks[u.ID] = make(map[string]bool)
	}
	if req.Owned {
		userSoulbreaks[u.ID][req.SoulbreakID] = true
	} else {
		delete(userSoulbreaks[u.ID], req.SoulbreakID)
	}
	userLock.Unlock()

	saveUserSoulbreaksForUser(u.ID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func userSoulbreaksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userSoulbreaksGetHandler(w, r)
	case http.MethodPost:
		userSoulbreaksPostHandler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func partySyncHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := r.FormValue("api_key")
	partyList := r.FormValue("party_list")
	if apiKey == "" || partyList == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing api_key or party_list")
		return
	}

	userLock.Lock()
	u := usersByAPI[apiKey]
	if u == nil {
		userLock.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "Invalid API key")
		return
	}
	userLock.Unlock()

	d := getAppDataSnapshot()
	if d == nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Data not loaded yet")
		return
	}

	// Parse the comma-separated values
	values := strings.Split(partyList, ",")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}

	var newParties []*SavedParty
	pos := 0
	for pos < len(values) {
		// Skip trailing empty values
		if pos+3 > len(values) {
			break
		}

		partyNumber := values[pos]
		partyName := values[pos+1]
		memberCountStr := values[pos+2]
		pos += 3

		memberCount, err := strconv.Atoi(memberCountStr)
		if err != nil || memberCount < 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Invalid member count: %s", memberCountStr)
			return
		}

		var charIDs []string
		var visibleSBs []string

		for m := 0; m < memberCount; m++ {
			if pos+21 > len(values) {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "Unexpected end of data while parsing party members")
				return
			}

			charID := values[pos]   // 1: Character ID (game buddy ID, needs mapping)
			// values[pos+1]        // 2: Record Materia ID (ignored)
			lm1 := values[pos+2]    // 3: Legend Materia 1
			lm2 := values[pos+3]    // 4: Legend Materia 2
			// values[pos+4]        // 5: Ability ID 1 (ignored)
			// values[pos+5]        // 6: Ability ID 2 (ignored)

			if lm1 != "" && lm1 != "0" {
				visibleSBs = append(visibleSBs, lm1)
			}
			if lm2 != "" && lm2 != "0" {
				visibleSBs = append(visibleSBs, lm2)
			}

			// Soulbreak IDs 1-15 (positions 6-20)
			for s := 6; s < 21; s++ {
				sbID := values[pos+s]
				if sbID != "" && sbID != "0" {
					visibleSBs = append(visibleSBs, sbID)
				}
			}

			charIDs = append(charIDs, charID)
			pos += 21
		}

		if len(charIDs) == 0 {
			continue
		}

		// Resolve game buddy IDs to CSV character IDs
		charIDs = resolveCharacterIDs(d, charIDs, visibleSBs)

		partyName = strings.TrimSpace(partyName)
		if len(partyName) > 60 {
			partyName = partyName[:60]
		}

		p := &SavedParty{
			ID:           "sync_" + partyNumber,
			Name:         partyName,
			CharacterIDs: charIDs,
			ShareKey:     generateID(),
			VisibleSBs:   visibleSBs,
			Source:       "sync",
		}
		newParties = append(newParties, p)
	}

	// Replace all synced parties atomically
	userLock.Lock()
	parties := userParties[u.ID]

	// Remove old synced parties and their share keys
	var kept []*SavedParty
	for _, p := range parties {
		if p.Source == "sync" {
			if p.ShareKey != "" {
				delete(partyByShare, p.ShareKey)
			}
		} else {
			kept = append(kept, p)
		}
	}

	// Add new synced parties
	for _, p := range newParties {
		for partyByShare[p.ShareKey] != 0 {
			p.ShareKey = generateID()
		}
		partyByShare[p.ShareKey] = u.ID
		kept = append(kept, p)
	}

	userParties[u.ID] = kept
	userLock.Unlock()

	saveUserPartiesForUser(u.ID)

	log.Printf("%s synced %d parties via ffrk_party_sync.", u.Username, len(newParties))
	fmt.Fprintf(w, "Synced %d parties to your account at https://ffrk.gigaforge.com", len(newParties))
}

func syncHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := r.FormValue("api_key")
	sbList := r.FormValue("sb_list")
	if apiKey == "" || sbList == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing api_key or sb_list")
		return
	}

	userLock.Lock()
	u := usersByAPI[apiKey]
	if u == nil {
		userLock.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "Invalid API key")
		return
	}

	if userSoulbreaks[u.ID] == nil {
		userSoulbreaks[u.ID] = make(map[string]bool)
	}

	// Build a set of known legend materia IDs for fallback matching
	lmIDs := make(map[string]bool)
	d := getAppDataSnapshot()
	if d != nil {
		for _, lmList := range d.LegendMateria {
			for _, lm := range lmList {
				lmIDs[lm.ID] = true
			}
		}
	}

	sbUpdates := 0
	lmUpdates := 0
	for _, id := range strings.Split(sbList, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !userSoulbreaks[u.ID][id] {
			userSoulbreaks[u.ID][id] = true
			if lmIDs[id] {
				lmUpdates++
			} else {
				sbUpdates++
			}
		}
	}
	userLock.Unlock()

	totalUpdates := sbUpdates + lmUpdates
	if totalUpdates > 0 {
		saveUserSoulbreaksForUser(u.ID)
		log.Printf("%s updated with %d new soulbreaks and %d new legend materia via FFRK-LabMem-SBTracker.", u.Username, sbUpdates, lmUpdates)
		var parts []string
		if sbUpdates > 0 {
			parts = append(parts, fmt.Sprintf("%d new Soulbreaks", sbUpdates))
		}
		if lmUpdates > 0 {
			parts = append(parts, fmt.Sprintf("%d new Legend Materia", lmUpdates))
		}
		fmt.Fprintf(w, "Added %s to your account at https://ffrk.gigaforge.com", strings.Join(parts, " and "))
	} else {
		fmt.Fprint(w, "No new soulbreaks to add")
	}
}
