package internal

import "fmt"

var RequiredBinaries = []string{"kubectl", "helm", "kind", "docker"}

func CheckPrerequisites(lookPath func(string) (string, error)) error {
	var missing []string
	for _, bin := range RequiredBinaries {
		if _, err := lookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required binaries: %v", missing)
	}
	return nil
}
