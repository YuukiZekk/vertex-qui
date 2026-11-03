// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Group is a named set of instances. An instance may be in any number of groups,
// and a feed that targets a group places on whatever instances are in it now.
type Group struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// InstanceIDs are the group's instances, in ascending order.
	InstanceIDs []int `json:"instanceIds"`
}

// ListGroups returns every group with its instances, by name.
func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM brush_groups ORDER BY name, id")
	if err != nil {
		return nil, fmt.Errorf("list brush groups: %w", err)
	}
	defer rows.Close()
	groups := []Group{}
	for rows.Next() {
		g := Group{InstanceIDs: []int{}}
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, fmt.Errorf("scan brush group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	members, err := s.GroupMembers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if ids := members[groups[i].ID]; ids != nil {
			groups[i].InstanceIDs = ids
		}
	}
	return groups, nil
}

// GroupMembers maps each group to its instances, in ascending order.
func (s *Store) GroupMembers(ctx context.Context) (map[int64][]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT group_id, instance_id FROM brush_instance_groups ORDER BY group_id, instance_id")
	if err != nil {
		return nil, fmt.Errorf("list brush group members: %w", err)
	}
	defer rows.Close()
	out := make(map[int64][]int)
	for rows.Next() {
		var group int64
		var instance int
		if err := rows.Scan(&group, &instance); err != nil {
			return nil, fmt.Errorf("scan brush group member: %w", err)
		}
		out[group] = append(out[group], instance)
	}
	return out, rows.Err()
}

func (s *Store) CreateGroup(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, "INSERT INTO brush_groups (name, created_at) VALUES (?, ?) RETURNING id",
		name, time.Now().Unix()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create brush group: %w", err)
	}
	return id, nil
}

func (s *Store) RenameGroup(ctx context.Context, id int64, name string) error {
	return s.execOne(ctx, "rename brush group", "UPDATE brush_groups SET name = ? WHERE id = ?", name, id)
}

// DeleteGroup removes a group, its memberships and every feed's reference to it.
// A feed left with no instance and no group is disabled, so it never falls back to
// placing on every instance.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete brush group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, "DELETE FROM brush_groups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete brush group: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("delete brush group %d: %w", id, ErrNotFound)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM brush_instance_groups WHERE group_id = ?", id); err != nil {
		return fmt.Errorf("delete brush group members: %w", err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT id, instance_ids, group_ids FROM brush_feeds WHERE group_ids <> ''")
	if err != nil {
		return fmt.Errorf("list brush feed groups: %w", err)
	}
	type feedGroups struct {
		id        int64
		instances string
		groups    []int64
	}
	var touched []feedGroups
	for rows.Next() {
		var f feedGroups
		var list string
		if err := rows.Scan(&f.id, &f.instances, &list); err != nil {
			rows.Close()
			return fmt.Errorf("scan brush feed groups: %w", err)
		}
		if f.groups, err = parseGroupIDs(list); err != nil {
			rows.Close()
			return fmt.Errorf("brush feed %d: %w", f.id, err)
		}
		if slices.Contains(f.groups, id) {
			f.groups = slices.DeleteFunc(f.groups, func(g int64) bool { return g == id })
			touched = append(touched, f)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, f := range touched {
		query := "UPDATE brush_feeds SET group_ids = ? WHERE id = ?"
		if f.instances == "" && len(f.groups) == 0 {
			query = "UPDATE brush_feeds SET group_ids = ?, enabled = 0 WHERE id = ?"
		}
		if _, err := tx.ExecContext(ctx, query, formatGroupIDs(f.groups), f.id); err != nil {
			return fmt.Errorf("drop group from brush feed %d: %w", f.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete brush group: %w", err)
	}
	return nil
}

// SetInstanceGroups replaces the groups an instance is in.
func (s *Store) SetInstanceGroups(ctx context.Context, instanceID int, groupIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set brush instance groups: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM brush_instance_groups WHERE instance_id = ?", instanceID); err != nil {
		return fmt.Errorf("clear brush instance groups: %w", err)
	}
	for _, g := range groupIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO brush_instance_groups (instance_id, group_id) VALUES (?, ?)", instanceID, g); err != nil {
			return fmt.Errorf("add brush instance group: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit brush instance groups: %w", err)
	}
	return nil
}

// InstanceGroups maps each instance to the groups it is in, in ascending order.
func (s *Store) InstanceGroups(ctx context.Context) (map[int][]int64, error) {
	members, err := s.GroupMembers(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int][]int64)
	for group, instances := range members {
		for _, id := range instances {
			out[id] = append(out[id], group)
		}
	}
	for id := range out {
		slices.Sort(out[id])
	}
	return out, nil
}

// formatGroupIDs stores a feed's groups as a comma-separated list.
func formatGroupIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

func parseGroupIDs(list string) ([]int64, error) {
	if list == "" {
		return []int64{}, nil
	}
	parts := strings.Split(list, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("group ids %q: %w", list, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
