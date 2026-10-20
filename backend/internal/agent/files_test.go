package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAgentFileValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		ok   bool
	}{{"图.png", testImage(), true}, {"renamed.pdf", testImage(), false}, {"bad.png", []byte("<script>bad</script>"), false}, {"notes.txt", []byte("hello"), true}, {"table.csv", []byte("year,revenue\n2026,100"), true}, {"data.json", []byte(`{"ok":true}`), true}, {"bad.json", []byte(`{`), false}, {"page.html", []byte("<html>"), false}, {"bad.txt", []byte{0xff}, false}, {"empty.txt", nil, false}, {"large.txt", make([]byte, 10<<20+1), false}} {
		_, _, _, e := fileInfo(tc.name, tc.data)
		if (e == nil) != tc.ok {
			t.Fatal(tc.name, e)
		}
	}
	if got := safeFileName("../../目录/报告\r\n.csv"); got != "报告.csv" {
		t.Fatal(got)
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, _ := z.Create("[Content_Types].xml")
	_, _ = f.Write([]byte("<Types/>"))
	f, _ = z.Create("xl/workbook.xml")
	_, _ = f.Write([]byte("<workbook/>"))
	_ = z.Close()
	if _, _, _, e := fileInfo("财务.xlsx", b.Bytes()); e != nil {
		t.Fatal(e)
	}
	if _, _, _, e := fileInfo("财务.docx", b.Bytes()); e == nil {
		t.Fatal("wrong OOXML package accepted")
	}
}
func TestPostgresFileAttachmentPersistenceAndContext(t *testing.T) {
	st := isolatedStore(t)
	ctx := context.Background()
	svc := New(st, &testExecutor{}, t.TempDir(), true)
	cv, e := svc.Create(ctx, 1, "files", Config{ModelKeyID: 1, Model: "text", Budget: "1"}, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	a, e := svc.UploadFile(ctx, 1, cv.ID, "年度报告.csv", []byte("季度,收入\nQ1,100\nQ2,120"))
	if e != nil {
		t.Fatal(e)
	}
	got, e := svc.LoadAsset(ctx, 1, a.ID)
	if e != nil || got.FileName != "年度报告.csv" || got.MIME != "text/csv" || !bytes.Equal(got.Data, []byte("季度,收入\nQ1,100\nQ2,120")) {
		t.Fatal(got, e)
	}
	if _, e = svc.LoadAsset(ctx, 2, a.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign asset access", e)
	}

	var office bytes.Buffer
	zw := zip.NewWriter(&office)
	f, _ := zw.Create("[Content_Types].xml")
	_, _ = f.Write([]byte("<Types/>"))
	f, _ = zw.Create("xl/workbook.xml")
	_, _ = f.Write([]byte("<workbook/>"))
	_ = zw.Close()
	sheet, e := svc.UploadFile(ctx, 1, cv.ID, "报表.xlsx", office.Bytes())
	if e != nil {
		t.Fatal("Office MIME persistence", e)
	}
	loadedSheet, e := svc.LoadAsset(ctx, 1, sheet.ID)
	if e != nil || loadedSheet.MIME != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatal(loadedSheet, e)
	}
	note := svc.attachmentNote(ctx, 1, cv.ID, []string{a.ID})
	if !strings.Contains(note, TextAssetPrefix+a.ID) || strings.Contains(note, "Q1,100") || strings.Contains(note, "可由 generate_image") {
		t.Fatal(note)
	}
	pdf, e := svc.UploadFile(ctx, 1, cv.ID, "说明.pdf", []byte("%PDF-1.7\nexample"))
	if e != nil {
		t.Fatal(e)
	}
	note = svc.attachmentNote(ctx, 1, cv.ID, []string{pdf.ID})
	if !strings.Contains(note, "未解析正文") {
		t.Fatal("claims unread document analysis")
	}
	img, e := svc.UploadFile(ctx, 1, cv.ID, "参考.png", testImage())
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(svc.attachmentNote(ctx, 1, cv.ID, []string{img.ID}), "可由 generate_image") {
		t.Fatal("lost image-reference compatibility")
	}

	run, e := svc.Submit(ctx, 1, cv.ID, "csv-message", "127.0.0.1", "请查看这份表格", []string{a.ID}, 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(run.History[0]), "Q1,100") || !strings.Contains(string(run.History[0]), TextAssetPrefix+a.ID) {
		t.Fatal("raw file cached in durable context")
	}
	in := Invocation{Kind: "text", Input: run.History}
	if e = svc.loadVisionInputs(ctx, run, &in); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(jsonBytes(ExpandTextAssets(in.Input, in.FileInputs))), "Q1,100") {
		t.Fatal("live file missing from dispatched context")
	}
	if _, e = st.DB.ExecContext(ctx, `UPDATE agent_assets SET expires_at=now()-interval '1 second' WHERE id=$1`, a.ID); e != nil {
		t.Fatal(e)
	}
	expired := Invocation{Kind: "text", Input: run.History}
	if e = svc.loadVisionInputs(ctx, run, &expired); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(jsonBytes(ExpandTextAssets(expired.Input, expired.FileInputs))), "Q1,100") {
		t.Fatal("expired original reloaded from history")
	}

	messages, e := st.Messages(ctx, 1, cv.ID)
	if e != nil || len(messages) != 1 || len(messages[0].Assets) != 1 || messages[0].Assets[0] != a.ID {
		t.Fatal("attachment missing from saved message", e)
	}
}
