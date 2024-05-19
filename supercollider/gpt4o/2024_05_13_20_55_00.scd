// Start the server
s.waitForBoot {
    // Define a SynthDef for a deep bass
    SynthDef(\deepBass, {
        |out = 0, freq = 55, amp = 0.5, pan = 0, attack = 0.01, decay = 0.2, sustain = 0.5, release = 1.0|

        var env = EnvGen.kr(
            Env.adsr(attack, decay, sustain, release), doneAction: 2
        );

        var osc = LFSaw.ar(freq, 0, 0.4) + SinOsc.ar(freq * 0.5, 0, 0.4);
        var filter = LPF.ar(osc, 100, 0.2);

        var mix = filter * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for a dark pad
    SynthDef(\darkPad, {
        |out = 0, freq = 220, amp = 0.3, pan = 0, attack = 0.5, decay = 2.0, sustain = 1.0, release = 3.0|

        var env = EnvGen.kr(
            Env.adsr(attack, decay, sustain, release), doneAction: 2
        );

        var osc1 = LFTri.ar(freq * 0.5, 0, 0.3);
        var osc2 = SinOsc.ar(freq, 0, 0.3);
        var osc3 = LFSaw.ar(freq * 1.5, 0, 0.3);

        var mix = Mix.new([osc1, osc2, osc3]) * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for a percussive kick drum
    SynthDef(\kick, {
        |out = 0, amp = 0.8, pan = 0|

        var env = EnvGen.kr(
            Env.perc(0.01, 0.2), doneAction: 2
        );

        var osc = SinOsc.ar(50, 0, 0.8) + LFTri.ar(50, 0, 0.4);
        var filter = LPF.ar(osc, 200, 0.5);
        var mix = filter * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for a snare drum
    SynthDef(\snare, {
        |out = 0, amp = 0.5, pan = 0|

        var env = EnvGen.kr(
            Env.perc(0.01, 0.2), doneAction: 2
        );

        var noise = WhiteNoise.ar * env;
        var osc = SinOsc.ar(200, 0, 0.2);
        var mix = Mix.new([noise, osc]) * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for hi-hats
    SynthDef(\hiHat, {
        |out = 0, amp = 0.3, pan = 0|

        var env = EnvGen.kr(
            Env.perc(0.005, 0.1), doneAction: 2
        );

        var noise = WhiteNoise.ar;
        var filter = BPF.ar(noise, 8000, 0.2);
        var mix = filter * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for a rhythmic synth line
    SynthDef(\rhythmicSynth, {
        |out = 0, freq = 440, amp = 0.2, pan = 0, attack = 0.01, decay = 0.2, sustain = 0.5, release = 1.0|

        var env = EnvGen.kr(
            Env.adsr(attack, decay, sustain, release), doneAction: 2
        );

        var osc = Pulse.ar(freq, 0.5, 0.3) + Saw.ar(freq * 0.5, 0.3);
        var filter = RLPF.ar(osc, 1000, 0.3);
        var mix = filter * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Patterns to generate the music
    Pdef(\bass, Pbind(
        \instrument, \deepBass,
        \dur, Pseq([1, 0.75, 0.5, 1.25, 1], inf),
        \freq, Pseq([55, 65, 45, 60], inf),
        \amp, 0.5,
        \attack, 0.01,
        \decay, 0.2,
        \sustain, 0.5,
        \release, 1.0,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;

    Pdef(\pad, Pbind(
        \instrument, \darkPad,
        \dur, Pseq([16, 12, 8, 16], inf),
        \freq, Pseq([220, 330, 440, 550], inf),
        \amp, 0.3,
        \attack, 0.5,
        \decay, 2.0,
        \sustain, 1.0,
        \release, 3.0,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;

    Pdef(\kick, Pbind(
        \instrument, \kick,
        \dur, Pseq([1, 0.5, 0.75, 1, 0.25], inf),
        \amp, 0.8,
        \pan, 0
    )).play;

    Pdef(\snare, Pbind(
        \instrument, \snare,
        \dur, Pseq([1, 1.5, 1, 0.5, 1], inf),
        \amp, 0.5,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;

    Pdef(\hiHat, Pbind(
        \instrument, \hiHat,
        \dur, Pseq([0.25, 0.5, 0.25, 0.5, 0.75, 0.25], inf),
        \amp, 0.3,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;

    Pdef(\rhythmicSynth, Pbind(
        \instrument, \rhythmicSynth,
        \dur, Pseq([0.5, 0.75, 0.25, 1, 1.5], inf),
        \freq, Pseq([440, 660, 330, 550, 770], inf),
        \amp, 0.2,
        \attack, 0.01,
        \decay, 0.2,
        \sustain, 0.5,
        \release, 1.0,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;
};
