
SynthDef.new("openai-gpt-4o-2024_12_17_08_54_40", { |out=0, amp=0.5|
    var sound;
    var tonal, noise, percussive, feedback, env, lfo1, lfo2, lfo3, mod1, mod2, mod3, reverb, delay, pitched, shaped, filtered, layered;
lfo1 = LFNoise2.kr(0.1).range(0.5, 2.0);
lfo2 = LFNoise0.kr(0.05).range(200, 1200);
lfo3 = LFNoise2.kr(0.03).range(300, 700);
mod1 = SinOsc.kr(lfo1).range(0.1, 0.9);
mod2 = SinOsc.kr(lfo2).range(0.5, 1.5);
mod3 = SinOsc.kr(lfo3).range(100, 400);

tonal = Mix.fill(4, { VarSaw.ar([lfo2, lfo3], 0, mod1) * 0.1 });
noise = PinkNoise.ar(0.05) ! 2;
percussive = Dust.ar(mod3) * Decay.ar(Impulse.ar(0.2), 0.4);
feedback = LocalIn.ar(2);

filtered = RLPF.ar(noise + feedback, lfo3, 0.1);
pitched = PitchShift.ar(filtered, 0.02, LFNoise2.kr(0.1).range(0.6, 1.5));
shaped = tanh(pitched) * 0.8;
layered = layered + shaped + tonal;

reverb = FreeVerb.ar(layered, 0.85, 0.1, 0.7);
delay = CombL.ar(reverb, 0.3, LFNoise2.kr(0.05).range(0.1, 0.3), 0.9);
env = EnvGen.ar(Env.asr(0.1, 1, 5), 1, doneAction: 2);

// Complex feedback and depth layers
LocalOut.ar([layered, delay] * 0.5);
sound = Limiter.ar(0.3 * Rotate2.ar(delay * mod2, reverb * mod3, lfo1));
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
