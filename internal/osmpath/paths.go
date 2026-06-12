package osmpath

import "path/filepath"

// Dir is the single on-disk folder for OSM fixtures and runtime map files.
const Dir = "testdata"

// PittsburghOSM is the local benchmark cache (downloaded on first benchmark run).
const PittsburghOSM = "testdata/Pittsburgh.osm"

// Join returns a safe path under Dir for a base filename (e.g. uploaded-*.osm).
func Join(name string) string {
	return filepath.Join(Dir, filepath.Base(name))
}
