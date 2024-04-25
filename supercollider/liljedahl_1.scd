(
s.waitForBoot({
	play {
		var p = LFPulse;
		var gain = 0.5;
		
		var output = tanh(
			p.ar([50, 52]) *
			p.kr([2, 1] / 4) +
			mean({|n|
				(
					p.ar(n * 300 + 99 * p.kr(2 - n / [1, 5, 7], (0..2) / 10).sum + 200) *
					p.kr(n + 1 * 6, 0, 0.8).lag
				)
			} ! 2) / 2
		);
		
		output * gain;
	}
});
)
