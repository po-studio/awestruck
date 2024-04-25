(
s.waitForBoot({
	play {
		var gain = 0.25;  
		var d = Duty;  
		
		var f = d.kr(
			1 / [1, 2, 3],  
			0,  
			Dseq([0, 3, 7, 12, 8] + 36, inf)  
		);
		
		GVerb.ar(
			Blip.ar(
				f.midicps * [1, 2, 3],  
				LFNoise1.kr(1, 8, 8)  
			).sum,  
			99,  
			4   
		) * gain;  
	}
});
)
