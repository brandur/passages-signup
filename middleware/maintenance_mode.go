package middleware

import (
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/brandur/passages-signup/ptemplate"
)

// MaintenanceModeMiddleware sits above the server stack and allows the entire
// service to be put into "maintenance mode" in which all API requests are
// termined with a 503 Service Unavailable. This in turn allows us to carry out
// critical maintenance on core infrastructure like the database without having
// to worry about load or writes.
type MaintenanceModeMiddleware struct {
	maintenanceMode bool
	renderer        *ptemplate.Renderer
}

func NewMaintenanceModeMiddleware(maintenanceMode bool, renderer *ptemplate.Renderer) *MaintenanceModeMiddleware {
	return &MaintenanceModeMiddleware{
		maintenanceMode: maintenanceMode,
		renderer:        renderer,
	}
}

func (m *MaintenanceModeMiddleware) Wrapper(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.maintenanceMode {
			w.WriteHeader(http.StatusServiceUnavailable)
			if err := m.renderer.RenderTemplate(w, "views/maintenance", map[string]interface{}{}); err != nil {
				logrus.Errorf("Error rendering maintenance mode: %v", err)
				_, _ = w.Write([]byte(fmt.Sprintf("Error rendering maintenance mode: %v", err)))
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}
