package signal

import "testing"

// 测试向量直接取自 python3：
//
//	python3 -c "import random; r=random.Random(1); print([r.random() for _ in range(5)])"
//
// 这几条断言是"寻优结果能与 study_fine.csv 逐格吻合"的地基：RNG 一旦漂移，
// 路径丢弃与 bootstrap 抽样全变，校准就无从谈起。
func TestPyRandomFloat64MatchesCPython(t *testing.T) {
	cases := []struct {
		seed int64
		want []float64
	}{
		{1, []float64{0.13436424411240122, 0.8474337369372327, 0.763774618976614, 0.2550690257394217, 0.49543508709194095}},
		{42, []float64{0.6394267984578837, 0.025010755222666936, 0.27502931836911926}},
		{12345, []float64{0.41661987254534116, 0.010169169457068361, 0.8252065092537432}},
	}
	for _, c := range cases {
		r := newPyRandom(c.seed)
		for i, want := range c.want {
			if got := r.Float64(); got != want {
				t.Fatalf("seed=%d 第 %d 个 random(): got %.17g want %.17g", c.seed, i, got, want)
			}
		}
	}
}

func TestPyRandomGetRandBitsMatchesCPython(t *testing.T) {
	r := newPyRandom(42)
	want7 := []uint32{81, 14, 3, 94, 35, 31, 28, 17}
	for i, w := range want7 {
		if got := r.GetRandBits(7); got != w {
			t.Fatalf("seed=42 第 %d 个 getrandbits(7): got %d want %d", i, got, w)
		}
	}
	r = newPyRandom(1)
	want32 := []uint32{577090037, 2444712010, 3639700191, 3445702192}
	for i, w := range want32 {
		if got := r.GetRandBits(32); got != w {
			t.Fatalf("seed=1 第 %d 个 getrandbits(32): got %d want %d", i, got, w)
		}
	}
}

// ChoiceIndex 是 bootstrap 重采样的唯一取样入口，必须与 random.choice 同序。
func TestPyRandomChoiceIndexMatchesCPython(t *testing.T) {
	r := newPyRandom(42)
	want := []int{10, 1, 0, 4, 3, 3, 2, 1, 10, 8, 1, 9}
	for i, w := range want {
		if got := r.ChoiceIndex(11); got != w {
			t.Fatalf("seed=42 第 %d 个 choice(range(11)): got %d want %d", i, got, w)
		}
	}
}
