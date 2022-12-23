


// Check to see if we have a server
var modeRequest = $.get("/api/shows");

// Remote 
modeRequest.success(function(result) {
	console.log("Connected to remote server", result);
	popToast("Connected to show server");
	mode = MODE_REMOTE;
	$(".LOCAL").remove();
	
	// Open default project
	// var projectArray = JSON.parse(result);
	var projectArray = result.map(show => show.id)
	var firstProject = projectArray[0];
});

modeRequest.error(function(result) {
	console.log("Remote server not found, entering local mode");
	popToast("No show server found; editor only");
	mode = MODE_LOCAL;
	$(".REMOTE").remove();
	
	// Open default headless project
	loadDefaultProject();
});

Timeline.init();

// Disable right clicks
$("body").on("contextmenu", "#timeline-canvas", function(e) {
	return false;
});

// Start rendering
timelineRenderUpdate();


// UI on clicks ******************************************

// Initialize
$(document).ready(function() {
	// Initialize MaterializeCSS things
	$('select').material_select();
	$('.modal').modal();
});

// OPEN MODAL
$("#btn-open-remote-modal").on("click", function() {
	updateRemoteProjectList();
});

// CREATE NEW PROJECT
$("#btn-new-project").on("click", function() {
	createNewProject();
});

// OPEN PROJECT
$("#btn-open-remote-project").on("click", function() {
	var toOpen = $("#select-remote-projects").val();
	openRemoteProject(toOpen);
});

// SAVE
$("#btn-save-remote").on("click", function() {
	saveRemoteProject();
});

// IMPORT PROJECT
$("#btn-upload-file").on("click", function() {
	openLocalFile("#input-project-upload", onProjectFileLoad);
});

// EXPORT PROJECT
$("#btn-export").on("click", function() {
	exportLocalProject();
});

// EXPORT SHOW
$("#btn-export-show").on("click", function() {
	exportLocalShowfile();
});

// UPLOAD AUDIO
$("#btn-music-upload").on("click", function() {
	uploadAudio($("#input-music-upload"));
});

// PLAY
$("#btn-play").on("click", function() {
	mediaPlayPause();
});

// PAUSE
$("#btn-stop").on("click", function() {
	mediaStop();
});

// CHANGE PLAYBACK SPEED
$("#select-rate").on("change", function() {
	let newRate = $("#select-rate").val();
	wavesurfer.setPlaybackRate(newRate);
});

// ALIGN KEYFRAMES
$("#btn-align").on("click", function() {
	Timeline.performAlign();
});

// EQUAL-SPACE KEYFRAMES
$("#btn-equal").on("click", function() {
	Timeline.performEqualSpace();
});

// REMOVE DUPLICATES
$("#btn-remove-dups").on("click", function() {
	Timeline.removeDuplicateKeyframes();
});

// TURN KEYFRAMES ON
$("#btn-set-on").on("click", function() {
	Timeline.setKeyframesOn();
});

function updateRemoteProjectList() {
	
	var projectDropdown = $("#select-remote-projects");
	projectDropdown.empty();
	projectDropdown.material_select();
	
	getRemoteProjectList(function(newList) {
		console.log(newList);
		
		for(let i = 0; i < newList.length; i++) {
			let newString = newList[i];
			
			let newOption = $("<option>").attr('value', newString).text(newString);
			
			projectDropdown.append(newOption);
			
			//console.log(newString);
		}
		
		// Refresh
		projectDropdown.material_select();
	});
}


document.addEventListener("keydown", function(e) {
	if(!$(document.activeElement).is("canvas")) {
		return;
	}
	
	var key = e.keyCode;
	
	if(key == KEY_SPACE) {
		mediaPlayPause(); 
	}
});

// ******************************************************
// WAVEFORM TESTING

function mediaPlayPause() {
	wavesurfer.playPause();
	
	if(wavesurfer.isPlaying()) {
		$("#icon-play").html("pause");
	}
	else {
		$("#icon-play").html("play_arrow");
	}
}

function mediaStop() {
	wavesurfer.stop();
	Timeline.time = 0;
	$("#icon-play").html("play_arrow");
}

//wavesurfer.load("./WizardsInWinter.mp3");

wavesurfer.on("ready", function() {
	//wavesurfer.play();
	wavesurfer.zoom(Timeline.timeScale);
	Timeline.duration = wavesurfer.getDuration();
});

wavesurfer.on("seek", function() {
	//Timeline.time = wavesurfer.getCurrentTime(); // Disabled seeking on audio since this wasn't working
})