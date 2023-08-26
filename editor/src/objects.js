
var Timeline = function () {
	this.name = "Global";
	this.fps = 120;
	this.actualfps = 30;
	this.lastTime = (new Date()).getTime();
}

function Track(id, keyframes) {
	this.id = id;
	this.keyframes = keyframes;
}

function Keyframe(channel, time, state) {
	this.channel = channel;
	this.time = time;
	this.oldTime = time;
	this.state = state;
	this.selected = false;
}

function KeyframeGroup(time) {
	this.time = time;
	this.keyframes = []; // keyframes;
}

function CrossTrackKeyframe(time, numTracks) {
	this.time = time;
	this.values = new Array(numTracks);
}

var wavesurfer = WaveSurfer.create({
	container: "#waveform-container",
	partialRender: true,
	audioRate: 1,
	autoCenter: false,
	hideScrollbar: true,
	// Colors
	cursorColor: "white"
});