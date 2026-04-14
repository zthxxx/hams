# CLI Architecture — Spec Delta (fix-v1-planning-gaps)

## MODIFIED

### `--hams-lucky` Flag Behavior

When the `--hams-lucky` flag is passed after a provider command, the CLI SHALL skip all interactive TUI prompts and auto-accept LLM-recommended tags and intro:

- The flag SHALL be extracted by `splitHamsFlags()` and propagated to the provider's `HandleCommand()`.
- The provider SHALL forward the lucky flag to the enrichment flow.
- `RunTagPicker()` SHALL accept a `lucky` parameter; when `true`, it SHALL return LLM-recommended tags immediately without displaying the TUI picker.
- `EnrichAsync()` SHALL auto-accept the LLM-generated intro when lucky mode is active.
- This behavior is equivalent to non-interactive TTY mode, but explicitly user-requested.

#### Scenario: --hams-lucky skips tag picker

Given the user runs `hams brew install git --hams-lucky`
When the LLM returns recommended tags `["development-tool", "vcs"]`
Then the tags SHALL be written to the Hamsfile without displaying the TUI picker
And the LLM-generated intro SHALL be written without confirmation.

#### Scenario: --hams-lucky with no LLM configured

Given the user runs `hams brew install git --hams-lucky` with no LLM CLI configured
When enrichment fails gracefully
Then the package SHALL still be installed and recorded without tags or intro
And a warning SHALL be logged about LLM unavailability.
