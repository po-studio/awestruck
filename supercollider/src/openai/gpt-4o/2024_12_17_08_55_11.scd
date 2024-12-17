
SynthDef.new("openai-gpt-4o-2024_12_17_08_55_11", { |out=0, amp=0.5|
    var sound;
    var tonalSources, noiseSources, impulseSources, feedbackLoop, modulationSources, filtered, reverb, delay, dynamicLFOs;
tonalSources = Mix.fill(4, { |i| 
    var freq = Rand(100, 1000);
    var mod = SinOsc.kr(Rand(0.1, 0.5), 0, Rand(0.1, 0.5), freq);
    var oscill = SinOsc.ar(mod + freq) * 0.25;
    transWave = SelectX.kr(Demand.kr(Dust.kr(Rand(0.1, 0.5)), 0, Dseq([0.5, 1, 2], inf)), [SinOsc.ar(mod + freq), 
                           Saw.ar(mod + freq)]);
    VarLag.kr(transWave * 0.5, 0.4) * oscill;
});
noiseSources = LPF.ar(WhiteNoise.ar(0.3), XLine.kr(Rand(100, 500), Rand(500, 2000), 20));
impulseSources = Resonz.ar(Dust.ar(Rand(1, 10), 0.1), Rand(200, 2000), 0.1);
modulationSources = LFDNoise3.kr([0.1, 0.2, 0.3]).range(200, 1200);
dynamicLFOs = [RLPF.ar(tonalSources, modulationSources, 0.5),
               BPF.ar(noiseSources, modulationSources, 0.2),
               Resonz.ar(impulseSources, modulationSources, 0.3)];
feedbackLoop = LocalIn.ar(2);
filtered = Splay.ar(dynamicLFOs + feedbackLoop, spread: 0.5);
reverb = GVerb.ar(filtered, roomsize: 50, revtime: 15, damping: 0.5, inputbw: 0.5, spread: 1);
delay = CombL.ar(reverb, 0.3, 0.3, 10);
sound = Limiter.ar(DelayN.ar(LocalOut.ar(Splay.ar([filtered + reverb + delay * 0.3] * 0.25)) + feedbackLoop, 0.02), 1.0);
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
