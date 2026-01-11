package ntpu

import (
	"testing"
)

// TestReverseMap tests if DepartmentCodes can be reversed properly
func TestReverseMap(t *testing.T) {
	t.Parallel()
	reverseMap := make(map[string]string)
	for name, code := range DepartmentCodes {
		reverseMap[code] = name
	}

	if len(reverseMap) != len(DepartmentCodes) {
		t.Errorf("Expected %d unique codes, got %d - possible duplicate codes", len(DepartmentCodes), len(reverseMap))
	}
}

// TestDepartmentCodeMappings tests department name lookup
func TestDepartmentCodeMappings(t *testing.T) {
	t.Parallel()
	// Test that the maps contain expected entries
	tests := []struct {
		name        string
		deptCode    string
		mapToCheck  string
		shouldExist bool
	}{
		{"公行系碼存在", "72", "DepartmentNames", true},
		{"社學系碼存在", "742", "DepartmentNames", true},
		{"社工系碼存在", "744", "DepartmentNames", true},
		{"企管碩士存在", "31", "MasterDepartmentNames", true},
		{"會計博士存在", "32", "PhDDepartmentNames", true},
		{"無效碼不存在", "99", "DepartmentNames", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var exists bool
			switch tt.mapToCheck {
			case "DepartmentNames":
				_, exists = DepartmentNames[tt.deptCode]
			case "MasterDepartmentNames":
				_, exists = MasterDepartmentNames[tt.deptCode]
			case "PhDDepartmentNames":
				_, exists = PhDDepartmentNames[tt.deptCode]
			}

			if exists != tt.shouldExist {
				t.Errorf("Expected existence=%v for code %q in %s, got %v",
					tt.shouldExist, tt.deptCode, tt.mapToCheck, exists)
			}
		})
	}
}

// TestExtractYear tests the year extraction logic
func TestExtractYear(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		studentID string
		expected  int
	}{
		{"9-digit format 107", "410712345", 107},
		{"9-digit format 128", "412812345", 128},
		{"8-digit format 10", "41012345", 10},
		{"8-digit format 99", "49912345", 99},
		{"Too short", "1234", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			year := ExtractYear(tt.studentID)
			if year != tt.expected {
				t.Errorf("Expected year %d, got %d", tt.expected, year)
			}
		})
	}
}

// TestDetermineDepartment tests department determination logic
func TestDetermineDepartment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		studentID  string
		department string
	}{
		// Law department (71x) - all return "法律系" regardless of group
		{"8-digit 學士 法律-法學 (712)", "41071201", "法律系"},  // 4+10+71+2+01 → 107年法律系法學組
		{"8-digit 學士 法律-司法 (714)", "41071401", "法律系"},  // 4+10+71+4+01 → 107年法律系司法組
		{"8-digit 學士 法律-財法 (716)", "41071601", "法律系"},  // 4+10+71+6+01 → 107年法律系財法組
		{"9-digit 學士 法律-法學 (712)", "410771201", "法律系"}, // 4+107+71+2+01 → 107年法律系法學組

		// Social departments (74x) - distinguish between 社學/社工
		{"8-digit 學士 社學 (742)", "41074201", "社學系"},  // 4+10+74+2+01 → 107年社學系
		{"8-digit 學士 社工 (744)", "41074401", "社工系"},  // 4+10+74+4+01 → 107年社工系
		{"9-digit 學士 社學 (742)", "410774201", "社學系"}, // 4+107+74+2+01 → 107年社學系
		{"9-digit 學士 社工 (744)", "410774401", "社工系"}, // 4+107+74+4+01 → 107年社工系

		// Regular departments (2-digit code only)
		{"8-digit 學士 企管 (79)", "41079001", "企管系"},
		{"8-digit 學士 資工 (85)", "41085001", "資工系"},
		{"9-digit 學士 電機 (87)", "410787001", "電機系"}, // 4+107+87+001 → 107年電機系

		// Graduate programs
		{"8-digit 碩士 企管 (31)", "71031001", "企業管理學系碩士班"},
		{"8-digit 博士 會計 (32)", "81032001", "會計學系博士班"},
		{"9-digit 碩士 法律 (51)", "710851001", "法律學系碩士班一般生組"}, // 7+108+51+001
		{"9-digit 博士 企管 (31)", "811131101", "企業管理學系博士班"},   // 8+111+31+101
		{"9-digit 博士 法律 (51)", "811151101", "法律學系博士班"},     // 8+111+51+101

		// Edge cases
		{"Too short", "410", "未知"},
		{"Empty string", "", "未知"},
		{"Invalid master", "77099999", "未知碩士班"},
		{"Invalid phd", "88099999", "未知博士班"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dept := determineDepartment(tt.studentID)
			if dept != tt.department {
				t.Errorf("Expected department %q, got %q", tt.department, dept)
			}
		})
	}
}

// TestGetDegreeTypeName tests degree type name extraction from student ID prefix
func TestGetDegreeTypeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		studentID string
		want      string
	}{
		// Valid degree types
		{"Continuing education (3)", "31247001", DegreeNameContinuing},
		{"Undergraduate (4)", "41247001", DegreeNameBachelor},
		{"Master (7)", "71247001", DegreeNameMaster},
		{"PhD (8)", "81247001", DegreeNamePhD},

		// 9-digit variants
		{"9-digit Continuing", "312470001", DegreeNameContinuing},
		{"9-digit Undergraduate", "412470001", DegreeNameBachelor},
		{"9-digit Master", "712470001", DegreeNameMaster},
		{"9-digit PhD", "812470001", DegreeNamePhD},

		// Edge cases
		{"Empty ID", "", DegreeNameUnknown},
		{"Invalid prefix 0", "01247001", DegreeNameUnknown},
		{"Invalid prefix 1", "11247001", DegreeNameUnknown},
		{"Invalid prefix 2", "21247001", DegreeNameUnknown},
		{"Invalid prefix 5", "51247001", DegreeNameUnknown},
		{"Invalid prefix 6", "61247001", DegreeNameUnknown},
		{"Invalid prefix 9", "91247001", DegreeNameUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GetDegreeTypeName(tt.studentID)
			if got != tt.want {
				t.Errorf("GetDegreeTypeName(%q) = %q, want %q", tt.studentID, got, tt.want)
			}
		})
	}
}

// BenchmarkExtractYear benchmarks the year extraction function
func BenchmarkExtractYear(b *testing.B) {
	studentID := "410812345"
	for b.Loop() {
		_ = ExtractYear(studentID)
	}
}

// BenchmarkDetermineDepartment benchmarks the department determination function
func BenchmarkDetermineDepartment(b *testing.B) {
	studentID := "410e12345"
	for b.Loop() {
		_ = determineDepartment(studentID)
	}
}
