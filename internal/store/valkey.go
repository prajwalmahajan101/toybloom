package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/valkey-io/valkey-go"
)

const appendStageLua = `
if redis.call('HGET', KEYS[1], 'stage_count') ~= ARGV[1] then
  return redis.call('HGET', KEYS[1], 'stage_count')
end
for i=2,#ARGV,2 do
  redis.call('HSET', KEYS[2], ARGV[i], ARGV[i+1])
end
return redis.call('HINCRBY', KEYS[1], 'stage_count', 1)
`

var stageScript = valkey.NewLuaScript(appendStageLua)

type ValkeyStore struct {
	client valkey.Client
}

func NewValkeyStore(client valkey.Client) *ValkeyStore {
	return &ValkeyStore{client: client}
}

func (v *ValkeyStore) SetBits(ctx context.Context, key string, offsets []uint64) error {
	if len(offsets) == 0 {
		return nil
	}
	cmds := make(valkey.Commands, len(offsets))
	for i, off := range offsets {
		cmds[i] = v.client.B().Setbit().Key(key).Offset(int64(off)).Value(1).Build()
	}
	for _, resp := range v.client.DoMulti(ctx, cmds...) {
		if err := resp.Error(); err != nil {
			return fmt.Errorf("valkeystore: setbit: %w", err)
		}
	}
	return nil
}

func (v *ValkeyStore) GetBits(ctx context.Context, key string, offsets []uint64) ([]bool, error) {
	if len(offsets) == 0 {
		return []bool{}, nil
	}
	cmds := make(valkey.Commands, len(offsets))
	for i, off := range offsets {
		cmds[i] = v.client.B().Getbit().Key(key).Offset(int64(off)).Build()
	}
	resps := v.client.DoMulti(ctx, cmds...)
	result := make([]bool, len(offsets))
	for i, resp := range resps {
		val, err := resp.AsInt64()
		if err != nil {
			return nil, fmt.Errorf("valkeystore: getbit: %w", err)
		}
		result[i] = val == 1
	}
	return result, nil
}

func (v *ValkeyStore) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	m, err := v.client.Do(ctx, v.client.B().Hgetall().Key(key).Build()).AsStrMap()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("valkeystore: hgetall: %w", err)
	}
	return m, nil
}

func (v *ValkeyStore) HSet(ctx context.Context, key string, fields map[string]string) error {
	if len(fields) == 0 {
		return nil
	}
	cmd := v.client.B().Hset().Key(key).FieldValue()
	for f, val := range fields {
		cmd = cmd.FieldValue(f, val)
	}
	if err := v.client.Do(ctx, cmd.Build()).Error(); err != nil {
		return fmt.Errorf("valkeystore: hset: %w", err)
	}
	return nil
}

func (v *ValkeyStore) Incr(ctx context.Context, key string) (int64, error) {
	val, err := v.client.Do(ctx, v.client.B().Incr().Key(key).Build()).AsInt64()
	if err != nil {
		return 0, fmt.Errorf("valkeystore: incr: %w", err)
	}
	return val, nil
}

func (v *ValkeyStore) SAdd(ctx context.Context, key string, members ...string) error {
	if len(members) == 0 {
		return nil
	}
	if err := v.client.Do(ctx, v.client.B().Sadd().Key(key).Member(members...).Build()).Error(); err != nil {
		return fmt.Errorf("valkeystore: sadd: %w", err)
	}
	return nil
}

func (v *ValkeyStore) SRem(ctx context.Context, key string, members ...string) error {
	if len(members) == 0 {
		return nil
	}
	if err := v.client.Do(ctx, v.client.B().Srem().Key(key).Member(members...).Build()).Error(); err != nil {
		return fmt.Errorf("valkeystore: srem: %w", err)
	}
	return nil
}

func (v *ValkeyStore) SMembers(ctx context.Context, key string) ([]string, error) {
	s, err := v.client.Do(ctx, v.client.B().Smembers().Key(key).Build()).AsStrSlice()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("valkeystore: smembers: %w", err)
	}
	return s, nil
}

func (v *ValkeyStore) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := v.client.Do(ctx, v.client.B().Del().Key(keys...).Build()).Error(); err != nil {
		return fmt.Errorf("valkeystore: del: %w", err)
	}
	return nil
}

func (v *ValkeyStore) AppendStage(ctx context.Context, metaKey string, expected int64, newStageFields map[string]string, newStageMetaKey string) (int64, error) {
	args := make([]string, 0, 1+len(newStageFields)*2)
	args = append(args, strconv.FormatInt(expected, 10))
	for f, val := range newStageFields {
		args = append(args, f, val)
	}
	resp := stageScript.Exec(ctx, v.client, []string{metaKey, newStageMetaKey}, args)
	val, err := resp.AsInt64()
	if err != nil {
		return 0, fmt.Errorf("valkeystore: append stage: %w", err)
	}
	return val, nil
}
