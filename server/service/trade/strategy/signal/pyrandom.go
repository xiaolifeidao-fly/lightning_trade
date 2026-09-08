package signal

// CPython 兼容的 Mersenne Twister。
//
// **为什么服务里会有一份自研 RNG**：r16 的降噪协议（每格 16 条路径、信号 5%
// 随机丢弃、情景 bootstrap）是从 docs/argus_single/backtest_capsf_study.py 移植
// 过来的，而那份脚本的产出 docs/argus_single/study_fine.csv 是本任务唯一的校准
// 金标准（需求大纲 §6.1 第 7 条：10 个精算格逐格吻合）。随机数一换，"哪些信号
// 被丢掉""bootstrap 抽到哪 30 天"就全变了，med/sign/p10 只能"大致接近"，
// 校准就从硬证据退化成"看上去差不多"。
//
// 用 math/rand 或 v2 都做不到这件事：它们的序列与 CPython 的
// random.Random 不同。所以这里按 CPython 的 _randommodule.c + random.py 逐行
// 实现三个原语，够用即止：
//
//	Float64()      ← random.random()
//	GetRandBits(k) ← random.getrandbits(k)（k ≤ 32）
//	ChoiceIndex(n) ← random.choice(seq) 的下标（_randbelow_with_getrandbits）
//
// 只用于研究口径的复现，不用于任何安全场景。测试向量见 pyrandom_test.go
// （直接取自本机 python3 的输出）。
type pyRandom struct {
	mt  [624]uint32
	idx int
}

// newPyRandom 等价于 CPython 的 random.Random(seed)：整数种子取绝对值后拆成
// 32 位小端字数组，走 init_by_array。
func newPyRandom(seed int64) *pyRandom {
	if seed < 0 {
		seed = -seed
	}
	key := make([]uint32, 0, 2)
	if seed == 0 {
		key = append(key, 0)
	}
	for v := uint64(seed); v > 0; v >>= 32 {
		key = append(key, uint32(v&0xffffffff))
	}
	r := &pyRandom{}
	r.initByArray(key)
	return r
}

func (r *pyRandom) initGenrand(s uint32) {
	r.mt[0] = s
	for i := uint32(1); i < 624; i++ {
		prev := r.mt[i-1]
		r.mt[i] = 1812433253*(prev^(prev>>30)) + i
	}
	r.idx = 624
}

func (r *pyRandom) initByArray(key []uint32) {
	r.initGenrand(19650218)
	i, j := 1, 0
	k := len(key)
	if k < 624 {
		k = 624
	}
	for ; k > 0; k-- {
		prev := r.mt[i-1]
		r.mt[i] = (r.mt[i] ^ ((prev ^ (prev >> 30)) * 1664525)) + key[j] + uint32(j)
		i++
		j++
		if i >= 624 {
			r.mt[0] = r.mt[623]
			i = 1
		}
		if j >= len(key) {
			j = 0
		}
	}
	for k = 623; k > 0; k-- {
		prev := r.mt[i-1]
		r.mt[i] = (r.mt[i] ^ ((prev ^ (prev >> 30)) * 1566083941)) - uint32(i)
		i++
		if i >= 624 {
			r.mt[0] = r.mt[623]
			i = 1
		}
	}
	r.mt[0] = 0x80000000
	r.idx = 624
}

// uint32n 一次 genrand_uint32（含 tempering）。
func (r *pyRandom) uint32n() uint32 {
	if r.idx >= 624 {
		const matrixA = 0x9908b0df
		const upperMask = 0x80000000
		const lowerMask = 0x7fffffff
		for i := 0; i < 624; i++ {
			y := (r.mt[i] & upperMask) | (r.mt[(i+1)%624] & lowerMask)
			next := r.mt[(i+397)%624] ^ (y >> 1)
			if y&1 != 0 {
				next ^= matrixA
			}
			r.mt[i] = next
		}
		r.idx = 0
	}
	y := r.mt[r.idx]
	r.idx++
	y ^= y >> 11
	y ^= (y << 7) & 0x9d2c5680
	y ^= (y << 15) & 0xefc60000
	y ^= y >> 18
	return y
}

// Float64 等价于 random.random()：取两个 32 位字凑 53 位有效位。
func (r *pyRandom) Float64() float64 {
	a := r.uint32n() >> 5
	b := r.uint32n() >> 6
	return (float64(a)*67108864.0 + float64(b)) * (1.0 / 9007199254740992.0)
}

// GetRandBits 等价于 random.getrandbits(k)，仅支持 k ∈ [1,32]
// （本协议用不到更宽的，超范围直接钳住而不是悄悄给个别的分布）。
func (r *pyRandom) GetRandBits(k uint) uint32 {
	if k == 0 {
		return 0
	}
	if k > 32 {
		k = 32
	}
	return r.uint32n() >> (32 - k)
}

// ChoiceIndex 等价于 random.choice(seq) 取到的下标：
// _randbelow_with_getrandbits —— 取 n 的位宽，拒绝采样到 < n。
func (r *pyRandom) ChoiceIndex(n int) int {
	if n <= 0 {
		return 0
	}
	k := bitLen(uint32(n))
	for {
		v := int(r.GetRandBits(k))
		if v < n {
			return v
		}
	}
}

func bitLen(v uint32) uint {
	n := uint(0)
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}
