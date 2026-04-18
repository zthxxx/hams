package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/i18n"
)

// PrintError outputs a UserFacingError in the appropriate format.
// In JSON mode, outputs a structured JSON object.
// In text mode, outputs human-readable error with suggestions.
func PrintError(err error, jsonMode bool) {
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		// Cycle 245: wrap plain errors via NewUserError so the
		// ErrorCode field is populated from errorCodeFromExit.
		// Pre-cycle-245 the fallback constructed a bare struct with
		// zero-value ErrorCode, and json:"error_code,omitempty"
		// stripped it from JSON output. CI/agent scripts that parse
		// error_code (per cli-architecture spec §"Error in JSON
		// mode") saw a missing field on plain-error paths, while
		// UserFacingError paths carried "GENERAL_ERROR". Same
		// fallback exit code (ExitGeneralError), but now the coarse
		// category always surfaces.
		ufe = hamserr.NewUserError(hamserr.ExitGeneralError, err.Error())
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

	fmt.Fprintf(os.Stderr, "%s%s\n", i18n.T(i18n.ErrorsPrefix), ufe.Message)
	for _, s := range ufe.Suggestions {
		fmt.Fprintf(os.Stderr, "%s%s\n", i18n.T(i18n.ErrorsSuggestionPrefix), s)
	}
}
