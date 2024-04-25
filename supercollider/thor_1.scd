(
s.waitForBoot({
	play {
		var x = SinOsc;  
		var y = LFNoise0;  
		var a = y.ar(8);
		
		var output = (
			x.ar(Pulse.ar(1) * 24) +  
			x.ar(90 + (a * 90)) +  
			MoogFF.ar(
				Saw.ar(y.ar(4, 333, 666)),  
				a * XLine.ar(1, 39, 99, 99, 0, 2)  
			)
		) ! 2 / 3;  
		
		output;  
	};
});
);