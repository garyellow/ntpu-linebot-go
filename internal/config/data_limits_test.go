package config

import "testing"

// TestDataLimitConstants ensures data limit constants are correctly defined
func TestDataLimitConstants(t *testing.T) {
	// Verify year ranges are logical
	if IDDataYearStart >= IDDataYearEnd {
		t.Errorf("IDDataYearStart (%d) should be less than IDDataYearEnd (%d)", IDDataYearStart, IDDataYearEnd)
	}

	if IDDataYearEnd >= IDDataCutoffYear {
		t.Errorf("IDDataYearEnd (%d) should be less than IDDataCutoffYear (%d)", IDDataYearEnd, IDDataCutoffYear)
	}

	if LMSLaunchYear >= IDDataYearStart {
		t.Errorf("LMSLaunchYear (%d) should be less than IDDataYearStart (%d)", LMSLaunchYear, IDDataYearStart)
	}

	if NTPUFoundedYear > LMSLaunchYear {
		t.Errorf("NTPUFoundedYear (%d) should be less than or equal to LMSLaunchYear (%d)", NTPUFoundedYear, LMSLaunchYear)
	}

	// Verify specific values (document expectations)
	if IDDataYearStart != 101 {
		t.Errorf("IDDataYearStart = %d, want 101", IDDataYearStart)
	}
	if IDDataYearEnd != 113 {
		t.Errorf("IDDataYearEnd = %d, want 113", IDDataYearEnd)
	}
	if IDDataCutoffYear != 114 {
		t.Errorf("IDDataCutoffYear = %d, want 114", IDDataCutoffYear)
	}
	if LMSLaunchYear != 94 {
		t.Errorf("LMSLaunchYear = %d, want 94", LMSLaunchYear)
	}
	if NTPUFoundedYear != 89 {
		t.Errorf("NTPUFoundedYear = %d, want 89", NTPUFoundedYear)
	}
}

// TestDataLimitMessages ensures messages are non-empty and well-formed
func TestDataLimitMessages(t *testing.T) {
	messages := map[string]string{
		"IDDataCutoffNotice":       IDDataCutoffNotice,
		"IDDataRangeHint":          IDDataRangeHint,
		"IDDataCutoffReason":       IDDataCutoffReason,
		"IDNotFoundWithCutoffHint": IDNotFoundWithCutoffHint,
		"IDYear114PlusMessage":     IDYear114PlusMessage,
		"IDYearTooOldMessage":      IDYearTooOldMessage,
		"IDYearBeforeNTPUMessage":  IDYearBeforeNTPUMessage,
		"IDYearFutureMessage":      IDYearFutureMessage,
	}

	for name, msg := range messages {
		if msg == "" {
			t.Errorf("%s should not be empty", name)
		}
		// Check minimum length (messages should be informative)
		if len(msg) < 10 {
			t.Errorf("%s = %q is too short, should be more informative", name, msg)
		}
	}
}

// TestFormatFunctions ensures format functions return expected output
func TestFormatFunctions(t *testing.T) {
	footer := FormatIDDataRangeFooter()
	if footer == "" {
		t.Error("FormatIDDataRangeFooter() should not return empty string")
	}
	if footer[0:1] != "\n" {
		t.Error("FormatIDDataRangeFooter() should start with newline for footer usage")
	}

	explanation := FormatIDCutoffExplanation()
	if explanation == "" {
		t.Error("FormatIDCutoffExplanation() should not return empty string")
	}
}
