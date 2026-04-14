package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	hamserr "github.com/zthxxx/hams/internal/error"
)

// PrintError outputs a UserFacingError in the appropriate format.
// In JSON mode, outputs a structured JSON object.
// In text mode, outputs human-readable error with suggestions.
func PrintError(err error, jsonMode bool) {
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		ufe = &hamserr.UserFacingError{
			Code:    hamserr.ExitGeneralError,
			Message: err.Error(),
		}
	}

	if jsonMode {
		data, jsonErr := json.MarshalIndent(ufe, "", "  ")
		if jsonErr != nil {
			fmt.Fprintf(os.Stderr, `{"code":%d,"message":"%s"}`+"\n", ufe.Code, ufe.Message)
			return
		}
		fmt.Fprintln(os.Stderr, string(data))
		return
	}

	fmt.Fprintf(os.Stderr, "Error: %s\n", ufe.Message)
	for _, s := range ufe.Suggestions {
		fmt.Fprintf(os.Stderr, "  suggestion: %s\n", s)
	}
}
