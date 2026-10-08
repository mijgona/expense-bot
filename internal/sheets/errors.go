package sheets

import (
	"errors"
	"net/http"
	"strings"

	"google.golang.org/api/googleapi"
)

// isMissingSheet reports a "range not found" error, which the Sheets API returns for absent tabs.
func isMissingSheet(err error) bool {
	var gerr *googleapi.Error
	return errors.As(err, &gerr) && gerr.Code == http.StatusBadRequest &&
		strings.Contains(gerr.Message, "Unable to parse range")
}
