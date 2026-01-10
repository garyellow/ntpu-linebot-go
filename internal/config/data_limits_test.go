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

	// CourseSystemLaunchYear should be between NTPUFoundedYear and LMSLaunchYear
	if CourseSystemLaunchYear < NTPUFoundedYear || CourseSystemLaunchYear > LMSLaunchYear {
		t.Errorf("CourseSystemLaunchYear (%d) should be between NTPUFoundedYear (%d) and LMSLaunchYear (%d)",
			CourseSystemLaunchYear, NTPUFoundedYear, LMSLaunchYear)
	}

	// Verify specific values (document expectations)
	if IDDataYearStart != 101 {
		t.Errorf("IDDataYearStart = %d, want 101", IDDataYearStart)
	}
	if IDDataYearEnd != 112 {
		t.Errorf("IDDataYearEnd = %d, want 112", IDDataYearEnd)
	}
	if IDDataCutoffYear != 113 {
		t.Errorf("IDDataCutoffYear = %d, want 113", IDDataCutoffYear)
	}
	if LMSLaunchYear != 94 {
		t.Errorf("LMSLaunchYear = %d, want 94", LMSLaunchYear)
	}
	if NTPUFoundedYear != 89 {
		t.Errorf("NTPUFoundedYear = %d, want 89", NTPUFoundedYear)
	}
	if CourseSystemLaunchYear != 90 {
		t.Errorf("CourseSystemLaunchYear = %d, want 90", CourseSystemLaunchYear)
	}
}

// TestDataLimitMessages ensures messages are non-empty and well-formed
func TestDataLimitMessages(t *testing.T) {
	messages := map[string]string{
		"IDLMSDeprecatedMessage":   IDLMSDeprecatedMessage,
		"ID113YearWarningMessage":  ID113YearWarningMessage,
		"ID113YearEmptyMessage":    ID113YearEmptyMessage,
		"IDNotFoundWithCutoffHint": IDNotFoundWithCutoffHint,
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
