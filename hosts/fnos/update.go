package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Updating executable files never rolls back or deletes the live user data tree.
// The backup is for manual migration recovery, independently of binary rollback.
func (m *Manager) updateFrom(folder string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.toneBusy() {
		return errors.New("请先停止测试音再安装或更新内核")
	}
	candidate, err := m.installRelease(folder)
	if err != nil {
		return err
	}
	old := m.active
	_, endpointErr := m.endpoint()
	running := endpointErr == nil
	backup := filepath.Join(m.Data, "backups", time.Now().UTC().Format("20060102T150405.000000000"))
	if err = backupUserData(filepath.Join(m.Data, "engine"), backup); err != nil {
		return fmt.Errorf("备份失败，更新未开始: %w", err)
	}
	if err = m.stopLocked(); err != nil {
		return err
	}
	if err = writeJSON(filepath.Join(m.Data, "active-engine.json"), candidate); err != nil {
		if running {
			_ = m.startLocked()
		}
		return err
	}
	m.active = candidate
	// Validate a candidate even when the old Engine was stopped, then restore its state.
	err = m.startLocked()
	if err == nil && !running {
		err = m.stopLocked()
	}
	if err == nil {
		return nil
	}
	cause := err
	if stopErr := m.stopLocked(); stopErr != nil {
		return fmt.Errorf("新版失败: %v；停止新版失败: %v。保留所有数据和备份，拒绝并行启动旧版", cause, stopErr)
	}
	if old.Binary == "" {
		if err = os.Remove(filepath.Join(m.Data, "active-engine.json")); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("首次安装失败，清理版本指针失败: %w", err)
		}
		m.active = old
		return fmt.Errorf("首次安装失败，保持未安装状态；用户数据保留: %w", cause)
	}
	if err = writeJSON(filepath.Join(m.Data, "active-engine.json"), old); err != nil {
		return fmt.Errorf("新版失败: %v；恢复版本指针失败: %w", cause, err)
	}
	m.active = old
	if running {
		if err = m.startLocked(); err != nil {
			return fmt.Errorf("新版失败: %v；旧版重启失败: %w。用户数据和备份均已保留", cause, err)
		}
	}
	return fmt.Errorf("更新失败，内核文件已回滚，未覆盖用户数据: %w", cause)
}
func backupUserData(source, target string) error {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		// Cache/logs are not identity/configuration. Preserve all other regular files.
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(target, rel), 0700)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(filepath.Join(target, rel), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		if err == nil {
			err = out.Sync()
		}
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
func unpackUpdate(path, dest string) error {
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer z.Close()
	expected := map[string]bool{"engine.run": false, "manifest.json": false, "manifest.sig": false}
	for _, f := range z.File {
		seen, ok := expected[f.Name]
		if !ok || seen || !f.Mode().IsRegular() {
			return errors.New("更新包只允许 engine.run、manifest.json、manifest.sig，且不得重复")
		}
		limit := int64(384 << 20)
		if f.Name != "engine.run" {
			limit = 64 << 10
		}
		if f.UncompressedSize64 > uint64(limit) {
			return errors.New("更新文件超出限制")
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(filepath.Join(dest, f.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			r.Close()
			return err
		}
		n, err := io.Copy(w, io.LimitReader(r, limit+1))
		r.Close()
		ce := w.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
		}
		if n > limit {
			return errors.New("解压大小超限")
		}
		expected[f.Name] = true
	}
	for _, seen := range expected {
		if !seen {
			return errors.New("更新包文件不完整")
		}
	}
	return nil
}
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 384<<20)
	folder, err := os.MkdirTemp(s.m.Data, "update-")
	if err != nil {
		jsonOut(w, 500, map[string]any{"error": err.Error()})
		return
	}
	defer os.RemoveAll(folder)
	archive, err := os.CreateTemp(s.m.Data, "upload-*.zip")
	if err != nil {
		jsonOut(w, 500, map[string]any{"error": err.Error()})
		return
	}
	defer os.Remove(archive.Name())
	_, err = io.Copy(archive, r.Body)
	ce := archive.Close()
	if err == nil {
		err = ce
	}
	if err == nil {
		err = unpackUpdate(archive.Name(), folder)
	}
	if err == nil {
		err = s.m.updateFrom(folder)
	}
	if err != nil {
		jsonOut(w, 422, map[string]any{"error": err.Error()})
		return
	}
	jsonOut(w, 200, s.m.snapshot())
}

// Kept bounded and called separately from API requests; never rename live stdout files.
func boundLogs(ctx context.Context, paths ...string) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, path := range paths {
				f, e := os.OpenFile(path, os.O_RDWR, 0600)
				if e != nil {
					continue
				}
				st, e := f.Stat()
				if e == nil && st.Size() > 8<<20 {
					if _, e = f.Seek(-(2 << 20), io.SeekEnd); e == nil {
						b, e := io.ReadAll(io.LimitReader(f, 2<<20))
						if e == nil {
							if e = f.Truncate(0); e == nil {
								_, _ = f.WriteAt(b, 0)
							}
						}
					}
				}
				f.Close()
			}
		}
	}
}
