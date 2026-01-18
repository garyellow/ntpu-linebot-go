package syllabus

import (
	"testing"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

func TestMatchProgramTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		fullNames []string
		rawReqs   []storage.RawProgramReq
		want      []storage.ProgramRequirement
	}{
		{
			name:      "empty inputs",
			fullNames: []string{},
			rawReqs:   []storage.RawProgramReq{},
			want:      nil,
		},
		{
			name:      "exact match",
			fullNames: []string{"商業智慧與大數據分析學士學分學程"},
			rawReqs: []storage.RawProgramReq{
				{Name: "商業智慧與大數據分析學士學分學程", CourseType: "必"},
			},
			want: []storage.ProgramRequirement{
				{ProgramName: "商業智慧與大數據分析學士學分學程", CourseType: "必"},
			},
		},
		{
			name:      "abbreviated match - raw name is substring",
			fullNames: []string{"商業智慧與大數據分析學士學分學程"},
			rawReqs: []storage.RawProgramReq{
				{Name: "商業智慧與大數據分析學程", CourseType: "選"},
			},
			want: []storage.ProgramRequirement{
				{ProgramName: "商業智慧與大數據分析學士學分學程", CourseType: "選"},
			},
		},
		{
			name:      "core match - handles degree prefix removal",
			fullNames: []string{"金融科技與量化金融學士學分學程"},
			rawReqs: []storage.RawProgramReq{
				{Name: "金融科技與量化金融學程", CourseType: "必"},
			},
			want: []storage.ProgramRequirement{
				{ProgramName: "金融科技與量化金融學士學分學程", CourseType: "必"},
			},
		},
		{
			name:      "no match defaults to elective",
			fullNames: []string{"智慧財產權學士學分學程"},
			rawReqs:   []storage.RawProgramReq{},
			want: []storage.ProgramRequirement{
				{ProgramName: "智慧財產權學士學分學程", CourseType: "選"},
			},
		},
		{
			name:      "filters non-program entries",
			fullNames: []string{"資工系3", "商業智慧與大數據分析學士學分學程"},
			rawReqs: []storage.RawProgramReq{
				{Name: "資工系3", CourseType: "必"},
				{Name: "商業智慧與大數據分析學程", CourseType: "選"},
			},
			want: []storage.ProgramRequirement{
				{ProgramName: "商業智慧與大數據分析學士學分學程", CourseType: "選"},
			},
		},
		{
			name:      "multiple programs with different types",
			fullNames: []string{"金融科技學士學分學程", "人工智慧學士學分學程"},
			rawReqs: []storage.RawProgramReq{
				{Name: "金融科技學程", CourseType: "必"},
				{Name: "人工智慧學程", CourseType: "選"},
			},
			want: []storage.ProgramRequirement{
				{ProgramName: "金融科技學士學分學程", CourseType: "必"},
				{ProgramName: "人工智慧學士學分學程", CourseType: "選"},
			},
		},
		{
			name:      "micro program (微學程) matching",
			fullNames: []string{"經濟資料科學學士微學程"},
			rawReqs: []storage.RawProgramReq{
				{Name: "經濟資料科學學士微學程", CourseType: "選"},
			},
			want: []storage.ProgramRequirement{
				{ProgramName: "經濟資料科學學士微學程", CourseType: "選"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MatchProgramTypes(tt.fullNames, tt.rawReqs)

			if len(got) != len(tt.want) {
				t.Errorf("MatchProgramTypes() returned %d items, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i].ProgramName != tt.want[i].ProgramName {
					t.Errorf("Program[%d].ProgramName = %q, want %q", i, got[i].ProgramName, tt.want[i].ProgramName)
				}
				if got[i].CourseType != tt.want[i].CourseType {
					t.Errorf("Program[%d].CourseType = %q, want %q", i, got[i].CourseType, tt.want[i].CourseType)
				}
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"商業智慧", "商業智慧"},
		{"商業 智慧", "商業智慧"},
		{"商業　智慧", "商業智慧"}, // full-width space
		{"  商業智慧  ", "商業智慧"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := normalizeName(tt.input); got != tt.want {
				t.Errorf("normalizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractCore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"商業智慧與大數據分析學士學分學程", "商業智慧與大數據分析"},
		{"金融科技與量化金融碩士學分學程", "金融科技與量化金融"},
		{"經濟資料科學學士微學程", "經濟資料科學"},
		{"資料拓析學士學分微學程", "資料拓析學士學分"}, // tricky case with nested terms
		{"AI學程", "AI"},
		{"純學程", "純"}, // edge case
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := extractCore(tt.input); got != tt.want {
				t.Errorf("extractCore(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestJaccardSimilarity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b     string
		minScore float64
	}{
		{"商業智慧學程", "商業智慧學程", 1.0},
		{"商業智慧學程", "業智慧學程", 0.8},   // high similarity
		{"商業智慧學程", "完全不同的東西", 0.0}, // low similarity expected
		{"", "商業智慧", 0.0},
		{"商業智慧", "", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			t.Parallel()
			got := jaccardSimilarity(tt.a, tt.b)
			if got < tt.minScore {
				t.Errorf("jaccardSimilarity(%q, %q) = %f, want >= %f", tt.a, tt.b, got, tt.minScore)
			}
		})
	}
}

func TestFindMatchingType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fullName string
		rawReqs  []storage.RawProgramReq
		want     string
	}{
		{
			name:     "exact match",
			fullName: "商業智慧學程",
			rawReqs:  []storage.RawProgramReq{{Name: "商業智慧學程", CourseType: "必"}},
			want:     "必",
		},
		{
			name:     "no match defaults to elective",
			fullName: "不存在的學程",
			rawReqs:  []storage.RawProgramReq{{Name: "商業智慧學程", CourseType: "必"}},
			want:     "選",
		},
		{
			name:     "substring match",
			fullName: "商業智慧與大數據分析學士學分學程",
			rawReqs:  []storage.RawProgramReq{{Name: "商業智慧與大數據分析學程", CourseType: "選"}},
			want:     "選",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := findMatchingType(tt.fullName, tt.rawReqs)
			if got != tt.want {
				t.Errorf("findMatchingType(%q, ...) = %q, want %q", tt.fullName, got, tt.want)
			}
		})
	}
}
