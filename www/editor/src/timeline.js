
Timeline.init = function() {
	
	this.projectData = {
		"name": "",
		"id": ""
	}
	
	// UI variables
	this.height = 400;
	this.timeBarHeight = 40;
	this.timeMarkerLabelSize = 14;
	this.toastDuration = 4000;
	
	this.sideBarWidth = 100;
	this.zoomBarHeight = 30;
	
	this.numTracks = 4; // This will be discarded when files are implemented
	this.trackHeight = 20;
	this.trackSpacing = 10;
	
	this.selectionTolerance = 10;
	this.selectionBoxStart = {"x": 0, "y": 0};
	this.selectionBoxEnd = {"x": 0, "y": 0};
	
	// Colors
	this.lineColor = "rgb(250, 250, 250)";
	this.trackColor = "rgb(122, 122, 122)";
	this.onKeyframeColor = "yellow";
	this.offKeyframeColor = "rgba(0,0,0,0)";
	this.keyframeOutline = "black";
	this.selectedKeyframeOutline = "red";
	this.selectionBoxColor = "#00ccff";
	this.songEndAreaColor = "rgba(0,0,0,0.5)";
	
	// Time
	this.time = 0;
	this.lastFrameTime = 0;
	this.duration = 10; // in seconds
	this.timePercent = 0; // Time in 0-1 format
	this.timeViewPosition = 0; // Time where the left of the view is
	this.timeScale = 100;
	this.timeScaleMin = 10;
	this.timeScaleMax = 400 ;
	
	this.timePanStartX = 0;
	this.timePanStartViewPosition = 0;
	
	this.keyframeDragStartX = -1;
	this.keyframeScalePivot = -1;
	this.keyframeScaleEnd = -1;
	
	// State
	this.state = {};
	this.state.draggingTime = false;
	this.state.draggingZoom = false;
	this.state.draggingSelection = false;
	this.state.draggingKeyframes = false;
	this.state.scalingKeyframes = false;
	this.state.draggingPan = false;
	this.state.holdingShift = false;
	this.state.holdingControl = false;

	// Undo
	this.undoBufferSize = 64;
	this.undoBuffer = new CBuffer(this.undoBufferSize);
	
	// Keyframing
	this.tracks = [];
	this.selectedKeyframes = [];
	this.activeTracks = [];
	this.duplicateKeyframeTolerance = 0.01;
	
	// Create canvas
	this.container = $("#timeline-container");
	this.canvas = document.createElement("canvas");
	this.canvas.id = "timeline-canvas";
	this.canvas.height = this.height;
	this.canvas.width = window.innerWidth;
	this.canvas.setAttribute("tabindex", 1);
	this.ctx = this.canvas.getContext("2d");
	this.container.append(this.canvas);
	
	// Build keyframes @TODO make this work with files?
	this.buildKeyframes();
	
	// Event listeners go here
	this.canvas.addEventListener("mousedown", this.mouseDown);
	this.canvas.addEventListener("click", this.mouseClicked);
	this.canvas.addEventListener("mousemove", this.mouseMoved);
	this.canvas.addEventListener("mouseup", this.mouseUp);
	document.addEventListener("mouseup", this.mouseUp);
	//this.canvas.addEventListener("mouseout", this.mouseUp);
	this.canvas.addEventListener("wheel", this.mouseWheel);
	this.canvas.addEventListener("keydown", this.keyDown);
	this.canvas.addEventListener("keyup", this.keyUp);
}

Timeline.getProjectObject = function() {
	
	var obj = {
		"projectData": this.projectData,
		"tracks": this.tracks
	};
	
	return obj;
}

Timeline.loadProjectObject = function(obj) {
	
	this.emptyUndoBuffer();
	
	this.projectData = obj.projectData;
	this.tracks = obj.tracks;
	
	updateProjectName(this.projectData.name);
}

Timeline.mouseDown = function(e) {
	
	var x = e.layerX;
	var y = e.layerY;
	var t = Timeline;
	
	var clicked = e.which;
	var rightClick = (e.which === 3);
	
	// @TODO Make this better
	// We don't want to be able to click while dragging
	if(t.state.draggingKeyframes) {
		if(clicked == LEFTCLICK) {
			t.stopDraggingKeyframes();
		}
		else if(clicked == RIGHTCLICK) {
			t.cancelDraggingKeyframes();
		}
		
		return; // Don't do anything else, just place the keyframes
	}
	
	if(t.state.scalingKeyframes) {
		if(clicked == LEFTCLICK) {
			t.stopScalingKeyframes();
		}
		else if(clicked == RIGHTCLICK) {
			t.cancelScalingKeyframes();
		}
		
		return;
	}
	
	// Time bar clicked
	if(x > t.sideBarWidth && y < t.timeBarHeight) {
		//t.time = t.xToTime(x);
		t.state.draggingTime = true;
		t.mouseMoved(e);
	}
	
	// Zoom bar clicked
	else if(x < t.sideBarWidth) {
		t.state.draggingZoom = true;
		t.mouseMoved(e);
	}
	
	// Track area clicked
	else if(x > t.sideBarWidth && y > t.timeBarHeight) {
		var clickedTrack = -1;
		clickedTime = t.xToTime(x);
		
		// Find clicked track
		for(let i = 0; i < t.tracks.length; i++) {
			let trackBot = t.timeBarHeight + (i+1) * (t.trackHeight + t.trackSpacing);
			let trackTop = trackBot - t.trackHeight;
			if(y < trackBot && y > trackTop) {
				clickedTrack = i;
				break;
			}
		}
		
		if(clicked == LEFTCLICK) {
			// Deselect everything before new selection if not holding shift
			if(!t.state.holdingShift) t.deselectAllKeyframes();
			var onTopOfKeyframe = false;
			
			if(clickedTrack != -1) {
				let k = t.findClosestKeyframe(clickedTime, clickedTrack);
				let tol = t.getSelectionTolerance();
				
				if(k != null) {
					if(isInside(k.time, clickedTime - tol, clickedTime + tol)) { // k.time < clickedTime + tol && k.time > clickedTime - tol) {
						onTopOfKeyframe = true;
						k.selected = !k.selected;
					}
				}
			}
			
			// Drag box selection if we didn't select a keyframe already
			if(!onTopOfKeyframe) {
				t.state.draggingSelection = true;
				t.selectionBoxStart.x = x;
				t.selectionBoxStart.y = y;
				t.selectionBoxEnd.x = x;
				t.selectionBoxEnd.y = y;
			}
		}
		
		else if(clicked == RIGHTCLICK) {
			// Add new keyframe
			if(clickedTrack != -1) {
				
				t.saveUndoState();
				var newTime = t.xToTime(x);
				var newState = t.state.holdingShift ? 1 : 0;
				
				// When holding control, add to all tracks
				if(t.state.holdingControl) {
					for(let i = 0; i < t.tracks.length; i++) {
						t.tracks[i].keyframes.push(new Keyframe(i, newTime, newState));
					}
				}
				else {
					t.tracks[clickedTrack].keyframes.push(new Keyframe(clickedTrack, t.xToTime(x), newState));
				}
				
				t.sortKeyframes();
			}
			
		}
		
		else if(clicked == MIDCLICK) {
			// Dragging (panning) the timeline
			t.timePanStartX = x;
			t.timePanStartViewPosition = t.timeViewPosition;
			t.state.draggingPan = true;
		}
	}
	
}

Timeline.mouseClicked = function(e) {
	
}

Timeline.mouseMoved = function(e) {
	var x = e.layerX;
	var y = e.layerY;
	var t = Timeline;
	
	if(t.state.draggingTime) {
		// Clamp X so we don't go off the edge
		let limitedX = (x < t.sideBarWidth) ? t.sideBarWidth : x;
		t.time = t.xToTime(limitedX);
		t.timePercent = t.time / t.duration;
		wavesurfer.seekTo(t.timePercent);
		console.log("Changed time", limitedX, t.time);
	}
	
	else if(t.state.draggingZoom) {
		//var zoomPercent = x / t.sideBarWidth;
		//t.timeScale = zoomPercent * 200;
	}
	
	else if(t.state.draggingSelection) {
		t.selectionBoxEnd.x = x;
		t.selectionBoxEnd.y = y;
		
		t.performBoxSelection();
	}
	
	else if(t.state.draggingPan) {
		var deltaX = t.timePanStartX - x;
		var deltaTime = deltaX / t.timeScale;
		var timelineWidth = (t.canvas.width - t.sideBarWidth) / t.timeScale;
		console.log("WIDTH", timelineWidth);
		var maxScroll = Math.max(0, t.duration - timelineWidth);
		t.timeViewPosition = Math.clamp(t.timePanStartViewPosition + deltaTime, 0, maxScroll);
		console.log("Setting ScrollLeft to:", t.timeViewPosition * t.timeScale);
		wavesurfer.setScroll(Math.ceil(t.timeViewPosition * t.timeScale));
	}
	
	// DRAGGING KEYFRAMES
	else if(t.state.draggingKeyframes) {
		
		if(t.keyframeDragStartX == -1) t.keyframeDragStartX = x;
		
		var deltaX = x - t.keyframeDragStartX;
		var deltaTime = deltaX / t.timeScale;
		
		var selKeys = t.selectedKeyframes;
		
		for(let i = 0; i < selKeys.length; i++) {
			var k = selKeys[i];
			
			k.time = k.oldTime + deltaTime;
		}
	}
	
	// SCALING KEYFRAMES
	else if(t.state.scalingKeyframes) {
		
		if(t.keyframeDragStartX == -1) t.keyframeDragStartX = x;
		var deltaX = x - t.keyframeDragStartX;
		var deltaTime = deltaX / t.timeScale;
		
		var startSize = t.keyframeScaleEnd - t.keyframeScalePivot;
		var scaleSize = t.keyframeScalePivot + deltaTime;
		
		var selKeys = t.selectedKeyframes;
		for(let i = 0; i < selKeys.length; i++) {
			var k = selKeys[i];
			
			var pct = (k.oldTime - t.keyframeScalePivot) / startSize;
			var myDeltaTime = pct * scaleSize;
			
			k.time = t.keyframeScalePivot + myDeltaTime;
		}
	}
}

Timeline.mouseUp = function(e) {
	var t = Timeline;
	
	// Clear state variables
	t.state.draggingTime = false;
	t.state.draggingZoom = false;
	t.state.draggingSelection = false;
	t.state.draggingPan = false;
}

Timeline.mouseWheel = function(e) {
	var t = Timeline;
	var goingUp = (e.deltaY < 0);
	
	if(goingUp) {
		t.timeScale += 10;
	}
	else {
		t.timeScale -= 10;
	}
	
	t.timeScale = Math.clamp(t.timeScale, t.timeScaleMin, t.timeScaleMax);
	
	wavesurfer.zoom(t.timeScale);
	wavesurfer.setScroll(t.timeViewPosition * t.timeScale);
	
	console.log("Timescale", t.timeScale);
}

Timeline.keyDown = function(e) {
	// Cancel if the user is not in the canvas
	if(!$(document.activeElement).is("canvas")) {
		return;
	}
	
	// Switch based on key
	var key = e.keyCode;
	
	if(key == KEY_DEL) {
		console.log("delete");
		Timeline.deleteSelectedKeyframes();
	}
	
	if(key == KEY_A) {
		Timeline.performAlign();
	}
	
	if(key == KEY_D) {
		Timeline.duplicateKeyframes();
	}
	
	if(key == KEY_E) {
		Timeline.performEqualSpace();
	}
	
	if(key == KEY_G) {
		Timeline.startDraggingKeyframes();
	}
	
	if(key == KEY_I) {
		Timeline.performKeyframeInvert();
	}
	
	if(key == KEY_R) {
		Timeline.removeDuplicateKeyframes();
	}
	
	if(key == KEY_S) {
		Timeline.startScalingKeyframes();
	}

	if(key == KEY_Z) {
		if(Timeline.state.holdingControl) Timeline.undo();
	}
	
	if(key == KEY_SHIFT) {
		Timeline.state.holdingShift = true;
	}
	
	if(key == KEY_CTRL) {
		Timeline.state.holdingControl = true;
	}
}

Timeline.keyUp = function(e) {
	var key = e.keyCode;
	
	if(key == KEY_G) {
		//Timeline.stopDraggingKeyframes();
	}
	
	if(key == KEY_SHIFT) {
		Timeline.state.holdingShift = false;
	}
	
	if(key == KEY_CTRL) {
		Timeline.state.holdingControl = false;
	}
}

Timeline.drawGUI = function() {
	
	// Update width in case of resize
	this.canvas.width = window.innerWidth;
	
	var w = this.canvas.width;
	var h = this.canvas.height;
	
	var side = this.sideBarWidth;
	
	this.ctx.clearRect(0, 0, w, h);
	
	// Draw time bar
	this.ctx.beginPath();
	this.ctx.strokeStyle = this.lineColor;
	this.ctx.moveTo(side, this.timeBarHeight);
	this.ctx.lineTo(w, this.timeBarHeight);
	this.ctx.stroke();
	
	// Draw time markers
	var markerSpacing = this.getTimeMarkerSpacing();
	const firstMarker = Math.roundUp(this.timeViewPosition, markerSpacing);
	var currentMarker = firstMarker;
	do {
		var xpos = this.timeToX(currentMarker);
		var ypos = this.timeBarHeight / 3
		this.drawLine(xpos, 0, xpos, ypos);
		var labelSize = this.timeMarkerLabelSize + "px";
		var labelOffset = this.timeMarkerLabelSize + 2;
		this.drawText(currentMarker.toTimeString(), xpos, ypos + labelOffset, this.lineColor, labelSize);
		currentMarker += markerSpacing;
	} while (xpos < w);
	
	// Draw current time
	this.ctx.strokeStyle = this.lineColor;
	this.ctx.strokeRect(5, 5, side-10, this.timeBarHeight-10);
	
	this.drawText(this.time.toTimeString(true), 20, 10+16, this.lineColor, 16+"px");
	
	// Draw zoom area
	var zoomBarY = h - this.zoomBarHeight;
	this.drawLine(0, zoomBarY, side, zoomBarY, this.lineColor);
	
	// Draw channel bars
	for(let i = 0; i < this.tracks.length; i++) {
		// Draw bar
		this.ctx.fillStyle = this.trackColor;
		let yPos = this.timeBarHeight + this.trackSpacing + i * (this.trackHeight + this.trackSpacing);
		this.ctx.fillRect(side, yPos, w-side, this.trackHeight);
		
		// Draw label
		this.ctx.font = "16px arial";
		// Temporary: adjust label colors like lights
		if(this.activeTracks[i]) this.ctx.fillStyle = "white";
		else this.ctx.fillStyle = "black";
		
		//this.ctx.fillStyle = this.lineColor;
		this.ctx.fillText("Channel " + (i+1), 10, yPos + 16);
	}
	
	// Draw keyframes
	for(let i = 0; i < this.tracks.length; i++) {
		// For keyframes in track
		for(let j = 0; j < this.tracks[i].keyframes.length; j++) {
			var x = this.timeToX(this.tracks[i].keyframes[j].time);
			if(x >= side) {
				var y = this.getKeyframeY(i);
				var state = this.tracks[i].keyframes[j].state;
				var selected = this.tracks[i].keyframes[j].selected;
				var fillColor = (state) ? this.onKeyframeColor : this.offKeyframeColor;
				var lineColor = (selected) ? this.selectedKeyframeOutline : this.keyframeOutline;
				
				// Draw a diamond or a line depending on how zoomed out we are
				if(this.timeScale <= 20) this.drawKeyframeLine(x, y, 8, lineColor);
				else this.drawDiamond(x, y, 8, lineColor, fillColor);
			}
		}
	}
	
	// Draw greyed-out area where song ends
	var songEndX = this.timeToX(this.duration);
	if(songEndX < w) {
		let width = w - songEndX;
		let height = h - this.timeBarHeight;
		
		this.ctx.fillStyle = this.songEndAreaColor;
		this.ctx.fillRect(songEndX, this.timeBarHeight + 1, width, height);
	}
	
	// Draw time position indicator
	var timeX = this.timeToX(this.time);
	var timeArrowY = Math.round(this.timeBarHeight/2);
	if(timeX < side) { // Draw left arrow
		this.ctx.fillStyle = "#cc0000";
		this.ctx.beginPath();
		this.ctx.moveTo(side, timeArrowY);
		this.ctx.lineTo(side+8, timeArrowY+8);
		this.ctx.lineTo(side+8, timeArrowY-8);
		this.ctx.closePath();
		this.ctx.fill();
	}
	else if(timeX > w) { // Draw right arrow
		this.ctx.fillStyle = "#cc0000";
		this.ctx.beginPath();
		this.ctx.moveTo(w, timeArrowY);
		this.ctx.lineTo(w-8, timeArrowY+8);
		this.ctx.lineTo(w-8, timeArrowY-8);
		this.ctx.closePath();
		this.ctx.fill();
	}
	else {
		this.drawLine(timeX, 0, timeX, h, "#cc0000");
	}
	
	// Draw selection box
	if(this.state.draggingSelection) {
		var sizeX = this.selectionBoxEnd.x - this.selectionBoxStart.x;
		var sizeY = this.selectionBoxEnd.y - this.selectionBoxStart.y;
		
		this.ctx.strokeStyle = this.selectionBoxColor;
		this.ctx.beginPath();
		this.ctx.strokeRect(this.selectionBoxStart.x, this.selectionBoxStart.y, sizeX, sizeY);
		this.ctx.stroke();
	}
	
	// FOR DEBUGGING: Draw FPS
	this.drawText(this.actualfps, 5, h-5, "red", "24px");
}

function timelineRenderUpdate() {
	requestAnimationFrame(timelineRenderUpdate);
	
	Timeline.update();
}

Timeline.update = function() {
	
	Timeline.updateTime();
	
	Timeline.selectedKeyframes = Timeline.getSelectedKeyframes();
	
	Timeline.drawGUI();
	
	// DO DELTA TIME FPS STUFF
	var date = new Date();
	var now = date.getTime();
	var delta = now - Timeline.lastTime;
	
	Timeline.actualfps = Math.round(1000 / delta);
	
	Timeline.lastTime = now;
	//console.log(this.actualfps);
}

Timeline.updateTime = function() {
	// If the time has been changed
	if(this.lastFrameTime != this.time) {
		this.updateActiveTracks();
	}
	this.lastFrameTime = this.time;
	
	this.timePercent = this.time / this.duration;
	
	// if audio is playing update our position to match
	if(wavesurfer.isPlaying()) {
		this.time = wavesurfer.getCurrentTime();
		//this.updateActiveTracks();
	}
	
	//console.log(this.findClosestKeyframe(this.time, 0, true));
}

Timeline.updateActiveTracks = function() {
	
	if(this.activeTracks.length == 0) {
		for(let j = 0; j < this.tracks.length; j++) {
			this.activeTracks.push(0);
		}
	}
	
	for(let i = 0; i < this.tracks.length; i++) {
		
		let lastKeyframe = this.findClosestKeyframe(this.time, i, true);
		
		if(lastKeyframe != null) this.activeTracks[i] = lastKeyframe.state;
		else this.activeTracks[i] = 0;
	}
	
	console.log(this.activeTracks);
}

// Helper functions
Timeline.drawLine = function(x, y, xend, yend, color) {
	this.ctx.strokeStyle = color;
	this.ctx.beginPath();
	this.ctx.moveTo(x+0.5, y+0.5);
	this.ctx.lineTo(xend+0.5, yend+0.5);
	this.ctx.stroke();
}

Timeline.drawText = function(text, x, y, color, size) {
	this.ctx.font = size + " arial";
	this.ctx.fillStyle = color;
	this.ctx.fillText(text, x, y);
}

Timeline.drawDiamond = function(x, y, size, color, fillColor) {
	this.ctx.strokeStyle = color;
	this.ctx.beginPath();
	this.ctx.moveTo(x-size, y);
	this.ctx.lineTo(x, y-size);
	this.ctx.lineTo(x+size, y);
	this.ctx.lineTo(x, y+size);
	this.ctx.closePath();
	
	if(fillColor != "") {
		this.ctx.fillStyle = fillColor;
		this.ctx.fill();
	}
	
	this.ctx.stroke();
}

Timeline.drawKeyframeLine = function(x, y, size, color) {
	
	this.ctx.strokeStyle = color;
	this.ctx.beginPath();
	this.ctx.moveTo(x, y+size);
	this.ctx.lineTo(x, y-size);
	this.ctx.stroke();
}

Timeline.getKeyframeY = function(track) {
	var y = (this.timeBarHeight + this.trackSpacing) + track * (this.trackHeight + this.trackSpacing);
	return y + this.trackHeight / 2;
}

// @todo fix this
Timeline.timeToX = function(t) {
	return this.sideBarWidth + (t - this.timeViewPosition) * this.timeScale; // Replace 0 with time offset
}
Timeline.xToTime = function(x) {
	
	let timeArea = this.canvas.width - this.sideBarWidth;
	let xInArea = x - this.sideBarWidth;
	return xInArea / this.timeScale + this.timeViewPosition;  // Replace 0 with time offset
}

Timeline.getSelectionTolerance = function() {
	return this.selectionTolerance / this.timeScale;
}

// FOR DEBUGGING ONLY
Timeline.buildKeyframes = function() {
	
	for(let i = 0; i < this.numTracks; i++){
		
		// Build frames
		//var k = [new Keyframe(i, 1.5, 1)];
		var keys = [];
		
		// Build track
		var t = new Track(i, keys);
		
		this.tracks.push(t);
	}
	
	this.updateActiveTracks();
}

Timeline.sortKeyframes = function() {
	
	for(let i = 0; i < this.tracks.length; i++) {
		
		this.tracks[i].keyframes.sort(function(a, b) {
			return a.time - b.time;
		});
	}
}

Timeline.findClosestKeyframe = function(time, trackIndex, onlyBefore = false) {
	var t = this.tracks[trackIndex];
	var current = null;
	for (let i = 0; i < t.keyframes.length; i++) {
		
		var k = t.keyframes[i];
		
		if(current == null) {
			current = k;
		}
		else{
			let newdiff = Math.abs(k.time - time);
			let olddiff = Math.abs(current.time - time);
			
			if(onlyBefore && k.time > this.time) continue;
			if((newdiff < olddiff)) current = k;
		}
	}
	
	// Just make sure that we aren't going to return a keyframe after
	if(onlyBefore && (current != null) && this.time < current.time) current = null;
	
	return current;
}

Timeline.startDraggingKeyframes = function() {
	this.saveUndoState();
	Timeline.state.draggingKeyframes = true;
}

Timeline.stopDraggingKeyframes = function() {
	Timeline.keyframeDragStartX = -1;
	Timeline.state.draggingKeyframes = false;
	for(let i = 0; i < Timeline.selectedKeyframes.length; i++) {
		Timeline.selectedKeyframes[i].oldTime = Timeline.selectedKeyframes[i].time;
	}
	Timeline.sortKeyframes();
}

Timeline.cancelDraggingKeyframes = function() {
	Timeline.keyframeDragStartX = -1;
	Timeline.state.draggingKeyframes = false;
	for(let i = 0; i < Timeline.selectedKeyframes.length; i++) {
		Timeline.selectedKeyframes[i].time = Timeline.selectedKeyframes[i].oldTime;
	}
}

Timeline.startScalingKeyframes = function() {
	Timeline.saveUndoState();
	Timeline.state.scalingKeyframes = true;
	
	// Get starting point
	var first = 999;
	var last = 0;
	for(k in Timeline.selectedKeyframes) {
		if(Timeline.selectedKeyframes[k].time < first) first = Timeline.selectedKeyframes[k].time;
		if(Timeline.selectedKeyframes[k].time > last) last = Timeline.selectedKeyframes[k].time;
	}
	
	Timeline.keyframeScalePivot = first;
	Timeline.keyframeScaleEnd = last;
	
	console.log("FIRST", first);
}

Timeline.stopScalingKeyframes = function() {
	Timeline.keyframeDragStartX = -1;
	Timeline.keyframeScalePivot = -1;
	Timeline.state.scalingKeyframes = false;
	for(let i = 0; i < Timeline.selectedKeyframes.length; i++) {
		Timeline.selectedKeyframes[i].oldTime = Timeline.selectedKeyframes[i].time;
	}
	Timeline.sortKeyframes();
}

Timeline.cancelScalingKeyframes = function() {
	Timeline.keyframeDragStartX = -1;
	Timeline.keyframeScalePivot = -1;
	Timeline.state.scalingKeyframes = false;
	for(let i = 0; i < Timeline.selectedKeyframes.length; i++) {
		Timeline.selectedKeyframes[i].time = Timeline.selectedKeyframes[i].oldTime;
	}
}

Timeline.getTimeMarkerSpacing = function() {
	
	if(this.timeScale > 40) {
		return 1;
	}
	else if(this.timeScale > 20) {
		return 5;
	}
	
	return 10;
}

Timeline.getSelectedKeyframes = function() {
	var selectedKeyframes = [];
	
	for(let i = 0; i < this.tracks.length; i++) {
		var t = this.tracks[i];
		
		for(let j = 0; j < t.keyframes.length; j++) {
			if(t.keyframes[j].selected === true) {
				selectedKeyframes.push(t.keyframes[j]);
			}
		}
	}
	
	return selectedKeyframes;
}

Timeline.deleteSelectedKeyframes = function() {
	
	var undoStateSaved = false;
	
	for(let i = 0; i < this.tracks.length; i++) {
		
		var t = this.tracks[i];
		var toRemove = [];
		
		for(let j = 0; j < t.keyframes.length; j++) {
			if(t.keyframes[j].selected === true) {
				toRemove.push(j);
			}
		}

		if(toRemove.length && !undoStateSaved) {
			this.saveUndoState(); // Only save undo state if we actually had anything selected
			undoStateSaved = true;
		}
		
		for(let k = toRemove.length - 1; k >= 0; k--) {
			t.keyframes.splice(toRemove[k], 1);
		}
	}
}

Timeline.duplicateKeyframes = function() {
	this.saveUndoState();
	var newKeyframes = JSON.parse(JSON.stringify(this.selectedKeyframes));
	
	this.deselectAllKeyframes();
	
	for(let i = 0; i < newKeyframes.length; i++) {
		
		let k = newKeyframes[i];
		let t = k.channel;
		
		console.log("NEWKEYFRAME", k);
		
		k.selected = true;
		this.tracks[t].keyframes.push(k);
	}
	
	this.startDraggingKeyframes();
}

Timeline.removeDuplicateKeyframes = function() {

	var totalNumDups = 0;
	
	for(let i = 0; i < this.tracks.length; i++) {
		
		var t = this.tracks[i];
		var dups = [];
		
		for(let j = 0; j < t.keyframes.length; j++) {
			
			let k1 = t.keyframes[j];
			
			for(let l = j+1; l < t.keyframes.length; l++) {
				
				let k2 = t.keyframes[l]
				
				if(this.checkOverlap(k1, k2)) {
					if(k1.state) k2.state = 1;
					dups.push(j);
					break;
				}
			}
		}

		if(dups.length) this.saveUndoState();
		
		totalNumDups += dups.length;
		
		for(let m = dups.length - 1; m >= 0; m--) {
			t.keyframes.splice(dups[m], 1);
		}
		
		console.log("Duplicates:", dups);
	}
	
	console.log("Removed", totalNumDups, "duplicates");
	popToast("Removed " + totalNumDups + " duplicates");
}

Timeline.checkOverlap = function (k1, k2) {
	let tol = this.duplicateKeyframeTolerance;
	return Math.abs(k1.time - k2.time) < tol;
}

Timeline.deselectAllKeyframes = function() {
	for(let i = 0; i < this.tracks.length; i++) {
		
		var t = this.tracks[i];
		
		for(let j = 0; j < t.keyframes.length; j++) {
			t.keyframes[j].selected = false;
		}
	}
}

Timeline.performAlign = function() {
	this.saveUndoState();

	//@TODO replace with getSelectedKeyframes function
	var selectedKeyframes = [];
	var totalTime = 0;
	
	for(let i = 0; i < this.tracks.length; i++) {
		var t = this.tracks[i];
		for(let j = 0; j < t.keyframes.length; j++) {
			var k = t.keyframes[j];
			if(k.selected) {
				selectedKeyframes.push(k);
				totalTime += k.time;
			}
		}
	}
	
	var avgTime = totalTime / selectedKeyframes.length;
	
	for(let k = 0; k < selectedKeyframes.length; k++) {
		
		selectedKeyframes[k].time = avgTime;
		selectedKeyframes[k].oldTime = avgTime;
	}
}

Timeline.performEqualSpace = function() {
	
	var selectedKeyframes = this.getSelectedKeyframes();
	var startTime = 9999;
	var endTime = 0;
	
	if(selectedKeyframes.length < 3) return;
	
	this.saveUndoState();
	
	// Sort selection
	selectedKeyframes.sort(function(a, b) {
		return a.time - b.time;
	});
	
	// Find ends of selection
	for(let i = 0; i < selectedKeyframes.length; i++) {
		if(selectedKeyframes[i].time < startTime) startTime = selectedKeyframes[i].time;
		if(selectedKeyframes[i].time > endTime) endTime = selectedKeyframes[i].time;
	}
	
	console.log("Start time", startTime);
	console.log("End time", endTime);
	
	// Remove keyframes on ends
	for(let i = selectedKeyframes.length - 1; i >=0; i--) {
		
		if(selectedKeyframes[i].time == startTime || selectedKeyframes[i].time == endTime) {
			selectedKeyframes.splice(i, 1);
		}
	}
	
	// If all of our keyframes were at the same time we don't have any left
	if(selectedKeyframes.length == 0) return;
	
	// Get number of discrete times we need
	// @TODO take into account columns of equal times
	var numSteps = 0;// selectedKeyframes.length;
	
	// Get number of unique times
	var uniqueTimes = [];
	for(let i = 0; i < selectedKeyframes.length; i++) {
		let thisTime = selectedKeyframes[i].time;
		
		if(uniqueTimes.indexOf(thisTime) < 0 ) {
			uniqueTimes.push(thisTime);
		}
	}
	uniqueTimes.sort();
	numSteps = uniqueTimes.length;
	
	console.log("Unique times:", uniqueTimes);
	console.log("Num steps:", numSteps);
	
	// Put keyframes into sections based on unique times
	var arrayOfArrays = [];
	for(let u = 0; u < uniqueTimes.length; u++) {
		
		var newSection = [];
		
		for(let i = 0; i < selectedKeyframes.length; i++) {
			if(selectedKeyframes[i].time == uniqueTimes[u]) {
				newSection.push(selectedKeyframes[i]);
			}
		}
		
		arrayOfArrays.push(newSection);
	}
	
	// Get the equalized distance between each unique time
	var stepLength = (endTime - startTime) / (numSteps + 1);
	
	for(let i = 0; i < arrayOfArrays.length; i++) {
		let newTime = startTime + (i+1) * stepLength;
		
		for(let j = 0; j < arrayOfArrays[i].length; j++) {
			arrayOfArrays[i][j].time = newTime;
			arrayOfArrays[i][j].oldTime = newTime;
		}
	}	
}

Timeline.performKeyframeInvert = function() {
	this.saveUndoState();

	var selected = this.selectedKeyframes;
	
	console.log(selected);
	
	for(let i = 0; i < selected.length; i++) {
		
		selected[i].state = !selected[i].state;
	}
}

Timeline.setKeyframesOn = function() {

	var selK = this.getSelectedKeyframes();
	var newState = 1;

	for(let i = 0; i < selK.length; i++) {
		if(selK[i].state) newState = 0; break;
	}

	for(let j = 0; j < selK.length; j++) {
		selK[j].state = newState;
	}
}

Timeline.performBoxSelection = function() {
	
	var startX = this.selectionBoxStart.x;
	var startY = this.selectionBoxStart.y;
	var endX = this.selectionBoxEnd.x;
	var endY = this.selectionBoxEnd.y;
	
	for(let i = 0; i < this.tracks.length; i++) {
		var t = this.tracks[i];
		var trackY = this.getKeyframeY(i);
		// Check if we need to search this track
		//if(isInside(trackY, startY, endY)) {
			//console.log("Track", i, "is inside");
			
		for(let j = 0; j < t.keyframes.length; j++) {
			
			let k = t.keyframes[j];
			
			let tX = this.timeToX(k.time);
			
			k.selected = (isInside(tX, startX, endX) && isInside(trackY, startY, endY)) || (k.selected && this.state.holdingShift);
		}
	}
}

Timeline.undo = function() {

	/*if(this.undoHandler.buffer.length) {
		var newTrackState = this.undoHandler.stack.pop();
		console.log("Undo to:", newTrackState);
		this.tracks = newTrackState;
		popToast("Undo");
	}
	else {
		popToast("Nothing to undo");
	}*/
	
	var lastState = this.undoBuffer.pop();
	
	if(lastState != null) {
		this.tracks = lastState;
		popToast("Undo");
	}
	else {
		popToast("Nothing to undo");
	}
}

Timeline.saveUndoState = function() {
	var newState = JSON.parse(JSON.stringify(this.tracks));
	this.undoBuffer.push(newState);
	
	/*var s = this.undoHandler.stack;

	if(s.length < this.undoStackSize) {
		s.push(newState);
	}*/
}

Timeline.emptyUndoBuffer = function() {
	this.undoBuffer.empty();
	this.undoBuffer = new CBuffer(this.undoBufferSize);
}

Timeline.tracksToShowfile = function() {
	// All keyframes in sequence
	var allKeyframes = [];
	
	this.removeDuplicateKeyframes();
	
	// Combine all tracks
	for(let i = 0; i < this.tracks.length; i++) {
		allKeyframes = allKeyframes.concat(this.tracks[i].keyframes);
	}
	
	if(allKeyframes.length == 0) {
		popToast("You don't have anything to export");
		return;
	}
	
	// Sort by time
	allKeyframes.sort(function(a, b) {
		return a.time - b.time;
	});
	
	// Keyframes grouped by time
	var groupedKeyframes = [];
	var currentKeyframeGroup = new KeyframeGroup(0);
	
	// Assemble keyframes by time
	for(let i = 0; i < allKeyframes.length; i++) {
		
		let k = allKeyframes[i];
		let t = currentKeyframeGroup.time;
		
		// If this keyframe is the same time as the last keyframe
		if(isCloseTo(k.time, t, this.duplicateKeyframeTolerance)) {
			currentKeyframeGroup.keyframes.push(k);
		}
		// If it's a new time
		else {
			let newGroup = JSON.parse(JSON.stringify(currentKeyframeGroup));
			groupedKeyframes.push(newGroup);
			currentKeyframeGroup = new KeyframeGroup(k.time);
			currentKeyframeGroup.keyframes.push(k);
		}
	}
	
	// Push the last set of keyframes so we don't miss it!
	groupedKeyframes.push(currentKeyframeGroup);
	
	var finalKeyframes = [];
	var blankKeyframe = new CrossTrackKeyframe(0, this.tracks.length);
	
	for(let i = 0; i < groupedKeyframes.length; i++) {
		let g = groupedKeyframes[i];
		
		// Initialize new frame
		let newFrame = new CrossTrackKeyframe(g.time, this.tracks.length);
		
		// Get previous keyframe
		let prevFrame = (i == 0) ? null : finalKeyframes[i - 1];
		
		// Fill values with known new values
		for(let k = 0; k < g.keyframes.length; k++) {
			let channel = g.keyframes[k].channel;
			newFrame.values[channel] = (g.keyframes[k].state) ? 1 : 0;
			console.log("Set ", channel, " to ", newFrame.values[channel]);
		}
		
		// Fill empty values with previous frame's value
		for(let v = 0; v < newFrame.values.length; v++) {
			if(newFrame.values[v] == undefined) {
				
				if(prevFrame != null) {
					newFrame.values[v] = prevFrame.values[v];
				}
				else {
					newFrame.values[v] = 0;
				}
				
			}
		}
		
		finalKeyframes.push(newFrame);
	}
	
	var stringKeyframes = "";
	
	// Convert keyframes to string
	for(let i = 0; i < finalKeyframes.length; i++) {
		let k = finalKeyframes[i];
		let newString = k.time;
		
		for(let n = 0; n < k.values.length; n++) {
			newString += "," + k.values[n];
		}
		
		newString += "\n";
		
		stringKeyframes += newString;
	}
	
	//console.log(groupedKeyframes);
	//console.log(finalKeyframes);
	
	return stringKeyframes;
}

//Timeline.undoHandler.stack = [null, null, null, null];