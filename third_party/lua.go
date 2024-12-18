package third_party

// 删除对应的分布式锁, 但删除前会去取得该锁，取锁失败会直接返回
const LuaCheckAndDeleteDistributionLock = `
	local localKey = KEYS[1]
	local targetToken = ARGV[1]
	local getToken = redis.call("get", localKey)
	if (not getToken or getToken ~= targetToken) then
		return 0
	else
		return redis.call("del", localKey)
	end
`

// 刷新分布式锁的过期时间，但删除前会去取得该锁，取锁失败会直接返回
const LuaCheckAndExpireDistributionLock = `
	local localKey = KEYS[1]
	local targetToken = ARGV[1]
	local expire = ARGV[2]
	local getToken = redis.call("get", localKey)
	if (not getToken or getToken ~= targetToken) then
		return 0
	else
		return redis.call("expire", localKey, expire)
	end
`
