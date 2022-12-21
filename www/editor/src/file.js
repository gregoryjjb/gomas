/**
 * Handles file/project loading
 */

/*****************************************
 * LOCAL FILE UPLOAD/DOWNLOAD, GENERIC
 */

function downloadPlaintext(filename, text) {
  var element = document.createElement("a");
  element.setAttribute(
    "href",
    "data:text/plain;charset=utf-8," + encodeURIComponent(text)
  );
  element.setAttribute("download", filename);

  element.style.display = "none";
  document.body.appendChild(element);

  element.click();

  document.body.removeChild(element);
}

function openLocalFile(inputElement, onLoadHandler) {
  var fileObject = $(inputElement).get(0);

  var reader = new FileReader();
  if (fileObject.files.length) {
    var textFile = fileObject.files[0];
    // Read the file
    reader.readAsText(textFile);
    // When it's loaded, process it
    $(reader).on("load", onLoadHandler);
  } else {
    popToast("Please select a file", true);
  }
}

function onProjectFileLoad(e) {
  var fileContents = e.target.result;

  if (fileContents && fileContents.length) {
    try {
      var projectObj = JSON.parse(fileContents);
      Timeline.loadProjectObject(projectObj);
      $(".modal").modal("close"); // Close all modals
      popToast("Project loaded");
    } catch (e) {
      popToast("Project file is invalid", true);
    }
  } else {
    popToast("Error loading file", true);
  }
}

// NOT USED?
function uploadAudio(audioInputElement) {
  var files = audioInputElement.get(0).files;
  var file = URL.createObjectURL(files[0]);
  wavesurfer.load(file);
}

// Only used in "bodiless" mode (i.e. no server)
function loadDefaultProject() {
  var project_url = "./default_show/default.json";
  var audio_url = "./default_show/default.mp3";

  $.get(project_url, function (data) {
    Timeline.loadProjectObject(data);
  });

  wavesurfer.load(audio_url);

  popToast("Loading default project");
}

/*****************************************
 * LOCAL FILE UPLOAD/DOWNLOAD, SPECIFIC
 */

function exportLocalShowfile() {
  var showText = Timeline.tracksToShowfile();

  var saveName = Timeline.projectData.id;
  if (saveName == "") saveName = "untitled";

  downloadPlaintext(saveName + ".txt", showText);
}

function exportLocalProject() {
  var saveObj = Timeline.getProjectObject();
  var saveString = JSON.stringify(saveObj);

  var saveName = Timeline.projectData.id;
  if (saveName == "") saveName = "untitled";

  downloadPlaintext(saveName + ".json", saveString);
}

/*****************************************
 * REMOTE UPLOAD/DOWNLOAD
 */

// CREATE PROJECT
function createNewProject() {
  // Get number of tracks
  var numTracks = parseInt($("#input-num-channels").val());
  if (numTracks < 1) {
    popToast("Please enter the number of tracks");
    return;
  }

  // Get input name
  var newName = $("#input-new-project-name").val();
  if (newName == "") {
    popToast("Please fill in the project name");
    return;
  }

  // Get audio file
  var newAudioFile = $("#input-audio-upload").get(0).files[0];
  if ($("#input-audio-upload").val() == "") {
    popToast("Please select a song file");
    return;
  }

  // Create ID
  var newId = idify(newName);

  var newProjectData = {
    name: newName,
    id: newId,
  };

  // Create project object
  var newTracks = createTrackArray(numTracks);
  var newProjectObject = {
    projectData: newProjectData,
    tracks: newTracks,
  };

  if (mode === MODE_LOCAL) {
    Timeline.loadProjectObject(newProjectObject);
    var audioURL = URL.createObjectURL(newAudioFile);
    wavesurfer.load(audioURL);
  } else {
    var newProjectString = JSON.stringify(newProjectObject);
    var newProjectFile = new Blob([newProjectString], {
      type: "application/json",
    });

    // Create form data
    var formData = new FormData();
    formData.append("projectName", newName);
    formData.append("audioFile", newAudioFile);
    //formData.append("projectFile", newProjectFile);
    formData.append("projectText", newProjectString);

    var xhr = new XMLHttpRequest();
    // Add any event handlers here...
    xhr.open("POST", API_PATH + "createproject.php", true);

    xhr.onload = function () {
      popToast(xhr.response);
    };

    xhr.send(formData);
  }

  $(".modal").modal("close");
}

function openRemoteProject(newProject) {
  if (newProject != "") {
    popToast("Opening " + newProject);
  } else {
    popToast("Project not found");
    return;
  }

  const projectURL = `/shows/${newProject}`;

  // Load project file
  $.get(projectURL, function (data) {
    console.log(data);
    var projectObject = data;
    Timeline.loadProjectObject(projectObject);

    $(".modal").modal("close");
  });

	wavesurfer.load(`/audio/${newProject}`);
}

function saveRemoteProject() {
  var projectObject = Timeline.getProjectObject();

	fetch(`/shows/${Timeline.projectData.id}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(projectObject),
	}).then(res => {
		if (res.ok) {
			popToast("Saved");
		} else {
			throw new Error(res.statusText)
		}
	}).catch(err => {
		alert(`Save failed: ${err}`);
	});
}

function getRemoteProjectList(callbackFunction) {
  fetch("/shows")
    .then((res) => res.json())
    .then((data) => {
      console.log("shows data", data);
			projectArray = data.map(show => show.id);
			callbackFunction(projectArray)
    })
    .catch((err) => console.error(err));
}

function createTrackArray(numTracks) {
  numTracks = Math.clamp(numTracks, 1, 16);

  newTracks = [];

  for (let i = 0; i < numTracks; i++) {
    let keys = [];
    newTracks.push(new Track(i, keys));
  }

  return newTracks;
}
