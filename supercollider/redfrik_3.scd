(
s.waitForBoot({
	play {
		var gain = 0.25; 
		MoogFF.ar(
			LFTri.ar(
				CombN.ar(
					Duty.ar(
						1 / 8, 
						0, 
						Dseq(
							Dshuf(
								List.fib(16) % 8 * 99, 
								8 
							),
							inf 
						)
					),
					4, 
					4, 
					16 
				)
			) / 4, 
			LFTri.kr(1 / 16, 0, 2000, 3000) 
		) ! 2 * gain; 
	}
});
)
