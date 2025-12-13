package admin

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"sync"
)

type EnvService struct {
	path string
	mu   sync.Mutex
}

func NewEnvService(path string) *EnvService {
	return &EnvService{path: path}
}

func (s *EnvService) Read() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readNoLock()
}

func (s *EnvService) readNoLock() (map[string]string, error) {
	values := map[string]string{}
	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx >= 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			values[key] = value
		}
	}
	return values, scanner.Err()
}

func (s *EnvService) Update(updates map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.readNoLock()
	if err != nil {
		return err
	}
	for k, v := range updates {
		current[k] = v
	}
	keys := make([]string, 0, len(current))
	for key := range current {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		value := current[key]
		sb.WriteString(key)
		sb.WriteString("=")
		sb.WriteString(value)
		sb.WriteString("\n")
	}
	return os.WriteFile(s.path, []byte(sb.String()), 0o644)
}
