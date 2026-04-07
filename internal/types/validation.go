package types

import "fmt"

// ValidateApp checks an AppConfig for common errors.
func ValidateApp(app AppConfig) []string {
	var errs []string

	if app.Image == "" {
		errs = append(errs, fmt.Sprintf("app %q: image is required", app.Name))
	}

	for _, p := range app.Ports {
		if p.Number < 1 || p.Number > 65535 {
			errs = append(errs, fmt.Sprintf("app %q: port %d is out of range (1-65535)", app.Name, p.Number))
		}
		if p.Name == "" {
			errs = append(errs, fmt.Sprintf("app %q: port %d is missing a name", app.Name, p.Number))
		}
	}

	if app.Ingress != nil && app.Ingress.Host == "" {
		errs = append(errs, fmt.Sprintf("app %q: ingress host is required", app.Name))
	}

	if app.Replicas < 0 {
		errs = append(errs, fmt.Sprintf("app %q: replicas must be >= 0", app.Name))
	}

	return errs
}
