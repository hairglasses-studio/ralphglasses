package fleet

import (
	"crypto/md5"
	"encoding/binary"
	"hash/fnv"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ShardStrategy assigns a repo path to one of the available workers.
type ShardStrategy interface {
	Assign(repoPath string, workers []WorkerInfo) string // returns worker ID
}

// --- HashShardStrategy: consistent hash ring ---

// HashShardStrategy uses an FNV-32a consistent hash ring to map repo paths to
// workers. The ring is built from worker IDs: each worker gets multiple virtual
// nodes (replicas) spread around the 32-bit hash space to improve distribution
// evenness. When a repo is assigned, its path is hashed and mapped to the
// nearest clockwise worker on the ring.
type HashShardStrategy struct {
	Replicas int // virtual nodes per worker; 0 defaults to 64
}

// hashRingPoint is a single point on the consistent hash ring.
type hashRingPoint struct {
	hash     uint32
	workerID string
}

// Assign hashes repoPath and returns the ID of the nearest clockwise worker on
// the ring. Returns empty string if workers is empty.
func (h *HashShardStrategy) Assign(repoPath string, workers []WorkerInfo) string {
	if len(workers) == 0 {
		return ""
	}

	replicas := h.Replicas
	if replicas <= 0 {
		replicas = 64
	}

	ring := h.buildRing(workers, replicas)
	key := hashKey(repoPath)

	// Binary search for the first ring point >= key.
	idx := sort.Search(len(ring), func(i int) bool {
		return ring[i].hash >= key
	})
	// Wrap around if past the end.
	if idx >= len(ring) {
		idx = 0
	}
	return ring[idx].workerID
}

func (h *HashShardStrategy) buildRing(workers []WorkerInfo, replicas int) []hashRingPoint {
	ring := make([]hashRingPoint, 0, len(workers)*replicas)
	for _, w := range workers {
		points := ketamaPoints(w.ID, replicas)
		for _, pt := range points {
			ring = append(ring, hashRingPoint{
				hash:     pt,
				workerID: w.ID,
			})
		}
	}
	sort.Slice(ring, func(i, j int) bool {
		return ring[i].hash < ring[j].hash
	})
	return ring
}

// ketamaPoints generates virtual node hashes using the Ketama technique: MD5
// the key with a replica counter, then extract 4 independent 32-bit hashes
// from each 16-byte MD5 digest. This produces well-distributed points.
func ketamaPoints(key string, count int) []uint32 {
	points := make([]uint32, 0, count)
	md5Rounds := (count + 3) / 4 // each MD5 yields 4 points
	for i := 0; i < md5Rounds; i++ {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], uint32(i))
		digest := md5.Sum(append([]byte(key), buf[:]...))
		for j := 0; j < 4 && len(points) < count; j++ {
			h := binary.LittleEndian.Uint32(digest[j*4 : j*4+4])
			points = append(points, h)
		}
	}
	return points
}

// hashKey returns an FNV-32a hash of s.
func hashKey(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// --- ExplicitShardStrategy: manual pattern-to-worker mapping ---

// ExplicitShardStrategy maps repo path patterns to specific worker IDs. Patterns
// use filepath.Match syntax (e.g. "*/ralphglasses" or "hairglasses-studio/*").
// The first matching rule wins. If no rule matches, it falls back to an optional
// Fallback strategy (nil Fallback returns empty string).
type ExplicitShardStrategy struct {
	// Rules maps glob patterns to worker IDs. Evaluated in order.
	Rules []ShardRule

	// Fallback is used when no rule matches. Nil means no assignment.
	Fallback ShardStrategy
}

// ShardRule maps a glob pattern to a target worker.
type ShardRule struct {
	Pattern  string // filepath.Match glob (matched against repo path)
	WorkerID string
}

// Assign checks each rule in order. The pattern is matched against both the full
// repoPath and its base name (last path component) for convenience.
func (e *ExplicitShardStrategy) Assign(repoPath string, workers []WorkerInfo) string {
	base := filepath.Base(repoPath)

	// Build a set of valid worker IDs for validation.
	valid := make(map[string]bool, len(workers))
	for _, w := range workers {
		valid[w.ID] = true
	}

	for _, rule := range e.Rules {
		matchFull, _ := filepath.Match(rule.Pattern, repoPath)
		matchBase, _ := filepath.Match(rule.Pattern, base)
		if (matchFull || matchBase) && valid[rule.WorkerID] {
			return rule.WorkerID
		}
	}

	if e.Fallback != nil {
		return e.Fallback.Assign(repoPath, workers)
	}
	return ""
}

// --- ShardMap: tracks repo-to-worker assignments ---

// ShardMap maintains a bidirectional mapping between repos and worker IDs.
// All methods are concurrency-safe.
type ShardMap struct {
	mu       sync.RWMutex
	repoTo   map[string]string   // repo path -> worker ID
	workerTo map[string][]string // worker ID -> repo paths
	strategy ShardStrategy       // used by Rebalance
}

// NewShardMap creates an empty ShardMap with the given strategy for rebalancing.
func NewShardMap(strategy ShardStrategy) *ShardMap {
	return &ShardMap{
		repoTo:   make(map[string]string),
		workerTo: make(map[string][]string),
		strategy: strategy,
	}
}

// Assign maps a repo to a worker. If the repo was previously assigned to a
// different worker, the old assignment is removed.
func (m *ShardMap) Assign(repo, workerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove old assignment if present.
	if old, ok := m.repoTo[repo]; ok && old != workerID {
		m.removeRepoFromWorker(old, repo)
	}

	m.repoTo[repo] = workerID
	m.addRepoToWorker(workerID, repo)
}

// WorkerFor returns the worker assigned to a repo.
func (m *ShardMap) WorkerFor(repo string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.repoTo[repo]
	return id, ok
}

// ReposFor returns all repos assigned to a worker. Returns nil if none.
func (m *ShardMap) ReposFor(workerID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	repos := m.workerTo[workerID]
	if repos == nil {
		return nil
	}
	out := make([]string, len(repos))
	copy(out, repos)
	sort.Strings(out)
	return out
}

// Rebalance redistributes all known repos across the given workers using the
// ShardMap's strategy. Repos previously assigned to workers not in the new set
// are reassigned. Repos that the strategy maps to the same worker remain stable.
func (m *ShardMap) Rebalance(workers []WorkerInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.strategy == nil {
		return
	}

	// Build set of valid worker IDs.
	valid := make(map[string]bool, len(workers))
	for _, w := range workers {
		valid[w.ID] = true
	}

	// Collect all repos.
	repos := make([]string, 0, len(m.repoTo))
	for repo := range m.repoTo {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	// Clear worker->repos mapping (will rebuild).
	m.workerTo = make(map[string][]string, len(workers))

	// Reassign each repo.
	for _, repo := range repos {
		newWorker := m.strategy.Assign(repo, workers)
		if newWorker == "" {
			// Strategy couldn't assign (no workers); remove mapping.
			delete(m.repoTo, repo)
			continue
		}
		m.repoTo[repo] = newWorker
		m.addRepoToWorker(newWorker, repo)
	}
}

// AllAssignments returns a snapshot of all repo->worker mappings.
func (m *ShardMap) AllAssignments() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.repoTo))
	for k, v := range m.repoTo {
		out[k] = v
	}
	return out
}

// Len returns the number of assigned repos.
func (m *ShardMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.repoTo)
}

// removeRepoFromWorker removes a repo from a worker's list (must hold mu).
func (m *ShardMap) removeRepoFromWorker(workerID, repo string) {
	repos := m.workerTo[workerID]
	for i, r := range repos {
		if r == repo {
			m.workerTo[workerID] = append(repos[:i], repos[i+1:]...)
			return
		}
	}
}

// addRepoToWorker adds a repo to a worker's list if not already present (must hold mu).
func (m *ShardMap) addRepoToWorker(workerID, repo string) {
	for _, r := range m.workerTo[workerID] {
		if r == repo {
			return
		}
	}
	m.workerTo[workerID] = append(m.workerTo[workerID], repo)
}

// Unassign removes a repo from the shard map.
func (m *ShardMap) Unassign(repo string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if wid, ok := m.repoTo[repo]; ok {
		m.removeRepoFromWorker(wid, repo)
		delete(m.repoTo, repo)
	}
}

// WorkerRepoCount returns a map of workerID -> number of assigned repos.
func (m *ShardMap) WorkerRepoCount() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int, len(m.workerTo))
	for wid, repos := range m.workerTo {
		out[wid] = len(repos)
	}
	return out
}

// RepoPathNormalize returns a cleaned, slash-separated path suitable for
// consistent hashing. This ensures the same repo on different OS path styles
// hashes identically.
func RepoPathNormalize(path string) string {
	return strings.ReplaceAll(filepath.Clean(path), "\\", "/")
}
