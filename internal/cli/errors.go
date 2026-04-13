package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/zthxxx/hams/internal/cliutil"
)

// PrintError outputs a UserFacingError in the appropriate format.
// In JSON mode, outputs a structured JSON object.
// In text mode, outputs human-readable error with suggestions.
func PrintError(err error, jsonMode bool) {
	var ufe *cliutil.UserFacingError
	if !errors.As(err, &ufe) {
		ufe = &cliutil.UserFacingError{
			Code:    cliutil.ExitGeneralError,
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
