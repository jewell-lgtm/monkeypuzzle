package trackingclient

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
)

// Identity is called only by explicit tracking commands. The registry survives
// remote DELETE and is never initialized during ordinary mp workflows.
func Identity(dir, root, piece string) (tracking.Key, error) {
	var key tracking.Key
	if err := os.MkdirAll(dir, 0700); err != nil {
		return key, err
	}
	unlock, err := adapters.NewOSFS("").LockFile(filepath.Join(dir, "identity.lock"))
	if err != nil {
		return key, err
	}
	defer unlock()
	path := filepath.Join(dir, "identity.json")
	ids := map[string]string{}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &ids); err != nil {
			return key, fmt.Errorf("read tracking identities: %w", err)
		}
		if ids == nil {
			return key, fmt.Errorf("tracking identity registry must be an object")
		}
	} else if !os.IsNotExist(err) {
		return key, err
	}
	projectKey, _ := json.Marshal([]string{root})
	pieceKey, _ := json.Marshal([]string{root, piece})
	changed := false
	for _, name := range []string{"machine", string(projectKey), string(pieceKey)} {
		if _, ok := ids[name]; !ok {
			ids[name] = rand.Text()
			changed = true
		}
	}
	key = tracking.Key{MachineID: ids["machine"], ProjectID: ids[string(projectKey)], PieceID: ids[string(pieceKey)]}
	if err := key.Validate(); err != nil {
		return key, fmt.Errorf("invalid stored tracking identity: %w", err)
	}
	if !changed {
		return key, nil
	}
	data, err = json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return key, err
	}
	tmp, err := os.CreateTemp(dir, ".identity-*")
	if err != nil {
		return key, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return key, err
	}
	if closeErr != nil {
		return key, closeErr
	}
	return key, os.Rename(tmp.Name(), path)
}
