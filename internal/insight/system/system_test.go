package system

import "testing"

func TestParseStat(t *testing.T) {
	// user nice system idle iowait irq softirq steal = 100 0 50 800 50 0 0 0
	// total = 1000; idle = idle(800)+iowait(50) = 850; busy = 150.
	b := []byte("cpu  100 0 50 800 50 0 0 0\ncpu0 50 0 25 400 25 0 0 0\n")
	busy, total, ok := parseStat(b)
	if !ok || total != 1000 || busy != 150 {
		t.Fatalf("parseStat = busy %d total %d ok %v; want 150/1000/true", busy, total, ok)
	}
	if _, _, ok := parseStat([]byte("garbage\n")); ok {
		t.Fatal("malformed /proc/stat should not parse")
	}
	if _, _, ok := parseStat([]byte("cpu x y z q r\n")); ok {
		t.Fatal("non-numeric fields should not parse")
	}
}

func TestCPUDelta(t *testing.T) {
	c := New()
	if _, ok := c.cpuDelta(150, 1000); ok {
		t.Fatal("first tick must omit cpu (no predecessor)")
	}
	// busy +75, total +300 → 75/300 = 0.25 → 250 permille
	if v, ok := c.cpuDelta(225, 1300); !ok || v != 250 {
		t.Fatalf("cpuDelta = %d ok %v; want 250/true", v, ok)
	}
	// counter reset (total goes backwards) → omitted, not a wild number
	if _, ok := c.cpuDelta(10, 100); ok {
		t.Fatal("counter reset should omit cpu")
	}
}

func TestParseLoadavg(t *testing.T) {
	l1, l5, l15, ok := parseLoadavg([]byte("0.50 1.25 2.00 1/234 5678\n"))
	if !ok || l1 != 50 || l5 != 125 || l15 != 200 {
		t.Fatalf("parseLoadavg = %d/%d/%d ok %v; want 50/125/200/true", l1, l5, l15, ok)
	}
	if _, _, _, ok := parseLoadavg([]byte("0.1\n")); ok {
		t.Fatal("too few fields should not parse")
	}
}

func TestParseMeminfo(t *testing.T) {
	b := []byte("MemTotal:       1000 kB\nMemFree:  100 kB\nMemAvailable:    250 kB\n")
	// used = (1000-250)/1000 = 0.75 → 750 permille
	if used, ok := parseMeminfo(b); !ok || used != 750 {
		t.Fatalf("parseMeminfo = %d ok %v; want 750/true", used, ok)
	}
	if _, ok := parseMeminfo([]byte("MemTotal: 1000 kB\n")); ok {
		t.Fatal("missing MemAvailable should not parse")
	}
}
