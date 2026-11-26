// Package ports declares what the use cases need from the outside world:
// logging, event publishing and file storage. The infrastructure layer
// implements these interfaces and the dependency container wires them in, so
// the application layer never imports infrastructure packages.
package ports
