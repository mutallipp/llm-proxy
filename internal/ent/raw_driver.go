package ent

import "entgo.io/ent/dialect"

// RawDriver 返回 client 使用的底层 driver，供非 generated 数据迁移执行方言 SQL。
func RawDriver(client *Client) dialect.Driver { return client.driver }
