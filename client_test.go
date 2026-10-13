package main

import "testing"

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"обычное имя", "Иван", "Иван"},
		{"пробелы по краям", "  Иван  ", "Иван"},
		{"пустая строка", "", "anon"},
		{"только пробелы", "   ", "anon"},
		{"перенос строки", "Иван\nПётр", "Иван Пётр"},
		{"возврат каретки", "Иван\rПётр", "ИванПётр"},
		{"слишком длинный", "abcdefghijklmnopqrstuvwxyz0123456789", "abcdefghijklmnopqrstuvwxyz012345"},
		{"юникод", "日本語", "日本語"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}
