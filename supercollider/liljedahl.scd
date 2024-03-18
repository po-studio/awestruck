(
postln("SUPERCOLLIDER HAS STARTED");

s.boot; // Ensure the server is booted

// https://sccode.org/1-4Rn
(
SynthDef(\sawFilt, { |out = 0, freq = 440, amp = 0.1, gate = 1, cf = 100, wobble = 3, t_bd, t_sd, pw = 0.4|
	var base = Splay.ar(RLPF.ar(Pulse.ar(freq * [0.99,0.5,1.01],pw),cf.lag(0.05),0.3).madd(SinOsc.ar(wobble).range(0.5,4)).sin) * 0.5;
	var env = Linen.kr(gate, attackTime: 0.01, releaseTime: 0.5, doneAction: 2);
	var sig = base;
	var bd = tanh(Ringz.ar(LPF.ar(Trig.ar(t_bd,SampleDur.ir),1000),30,0.5,5).sin*2);
	var sd = tanh(Ringz.ar(LPF.ar(Trig.ar(t_sd,SampleDur.ir),1000),120,0.75,PinkNoise.ar(2!2)).sin*2);
	sd = HPF.ar(sd,60);
	sig = tanh(GVerb.ar(HPF.ar(base * env,30), 70, 11, 0.15)*0.5 + sig + bd + sd);
	Out.ar(out, sig*amp*env);
}).add;
);

s.waitForBoot({
	
	(
		p = Pproto(
			{
				~id = (
					type: \on,
					dur: 0,
					instrument: \sawFilt,
					amp: 1,
				).yield[\id];
				~type = \set;
			},
			Ppar([
				Pbind(
					\args, #[freq],
					\freq, Pseq([49,47,50,48]-12,inf).midicps,
					\dur, 4,
				),
				Pbind(
					\args, #[cf],
					\cf, Pseq([100,Prand([700,400,1100],3)],inf),
					\stutter, Prand([1,2,4,8],inf),
					\dur, PdurStutter(Pkey(\stutter)*2, 2),
					// NOTE: also useful is .collect on patterns!
					// \dur, Prand([1,2,4,8], inf).collect{ |x| (1/x)!(x*2) }.flatten
				),
				Pbind(
					\args, #[wobble],
					\wobble, Pxrand([3,1.5,6],inf),
					\dur, Prand([2,1],inf),
				),
				Pbind(
					\args, #[t_bd],
					\t_bd, Pseq([1,0,0,1],inf),
					\dur, 0.5,
				),
				Pbind(
					\args, #[t_sd],
					\t_sd, Pseq([0,0,1,0],inf),
					\dur, 0.5,
				),
				Pbind(
					\args, #[pw],
					\pw, Prand([0.4,0.3,0.5],inf),
					\dur, 2,
				),
			])
		).play;
	)
});
);