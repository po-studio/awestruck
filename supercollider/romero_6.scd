(
s.waitForBoot({
	play {
		var i = Impulse; 
		var a = Splay.ar(
			
			(PulseCount.ar(
				i.ar((1..8)), 
				i.ar(1 / 3) 
			) > 4) * 
			Blip.ar(
				(40, 44..62).midicps, 
				2 
			)
		);
		
		a * 0.1 + GVerb.ar(
			HPF.ar(a, 4e3), 
			300, 
			14   
		) * 4 
	};
});
)