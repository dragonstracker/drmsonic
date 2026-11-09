function getStreamId(id) {
	return /_.*_(.*)/.exec(id)[1]
}

function getProviderId(id) {
	return /_(.*)_.*/.exec(id)[1]
}

function getId(id) {
	if (id.includes('recstreamname')) {
		return /recstreamname(.*)/.exec(id)[1]
	} 	
	return /(_.*_.*)/.exec(id)[1]
}

function initPlayer() {
	document.getElementById('playerDiv').style.display = 'block';

	playerAudioSelect.onchange = playerAudioSelectOnchange;
	playerSubtitlesSelect.onchange = playerSubtitlesSelectOnchange;

	document.getElementById('player').onloadedmetadata = function() {
		if (!document.pictureInPictureEnabled) {
			return
		}
		if (!isInViewport(this)) {
			try {
				this.requestPictureInPicture();
			} catch(err) {
				console.error(err);
			}
		}
	}

}

async function noPlayer(e) {
	e.preventDefault();
	showStatusWindow('Stream not available');
}

async function setPlayer(e, id) {
	e.preventDefault();

	video = document.getElementById('player');
	if (!video) {
		return false
	}

	if (!id) {
		id = this.id
	}

	n = getId(id)

	if (document.getElementById('recstreamname' + n) != null) {
		src = document.getElementById('recstreamname' + n).href
	} else if (id.startsWith('replay')) {
		src = document.getElementById('replay' + n).href
	} else {
		src = document.getElementById('name' + n).href
	}

	if (!src.includes('http')) {
		return false
	}
	document.getElementById('trackselection').style.display = 'none';

	if (this.innerText) {
		document.getElementById('currentChannel').innerText = this.innerText;
	} else {
		document.getElementById('currentChannel').innerText = id;
	}
	document.getElementById('currentChannel').style.border = '1px solid white';

	const nativeHls = player.canPlayType('application/vnd.apple.mpegurl') !== '';
	const hlsSrc = src.includes('.m3u8');

	if (src.endsWith('.mp4') || (hlsSrc && !Hls.isSupported() && nativeHls)) {
		if (hls) {
			hls.destroy()
			hls = null
		}
		player.src = src;
		player.play().catch(function(){})
	} else if (Hls.isSupported()) {
		if (hls) {
			hls.destroy()
		}

		hlsjsConfig = {"maxBufferSize": 0, "maxBufferLength": 30, "liveSyncDurationCount": 3, "liveMaxLatencyDurationCount": 10, "manifestLoadingTimeOut": 20000, "manifestLoadingMaxRetry": 4, "levelLoadingMaxRetry": 4, "fragLoadingMaxRetry": 4, "debug": false};
		hls = new Hls(hlsjsConfig);
		hls.loadSource(src);
		hls.attachMedia(video);
		hls.on(Hls.Events.MANIFEST_PARSED,function() {
			video.play().catch(function(){})
		});
		hls.on(Hls.Events.ERROR,function(evt, data) {
			if (!data.fatal) {
				return
			}
			if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
				hls.startLoad()
			} else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
				hls.recoverMediaError()
			} else {
				hls.destroy()
				hls = null
				showStatusWindow('playback error: ' + data.details)
			}
		});
		hls.on(Hls.Events.AUDIO_TRACKS_UPDATED,function() {
			if ($("#playerAudioSelect")) {
				$("#playerAudioSelect").empty();
			}
			for (i=0; i<hls.audioTracks.length; i++) {
				opt = document.createElement("option");
				opt.value = i;
				opt.text = hls.audioTracks[i].name
				document.getElementById('playerAudioSelect').add(opt, null)
			}
			document.getElementById('trackselection').style.display = 'block';
		});
		hls.on(Hls.Events.SUBTITLE_TRACKS_UPDATED,function() {
			if ($("#playerSubtitlesSelect")) {
				$("#playerSubtitlesSelect").empty();
			}
			for (i=0; i<hls.subtitleTracks.length; i++) {
				opt = document.createElement("option");
				opt.value = i;
				opt.text = hls.subtitleTracks[i].name
				document.getElementById('playerSubtitlesSelect').add(opt, null)
			}
			opt = document.createElement("option");
			opt.value = -1;
			opt.text = "none"
			document.getElementById('playerSubtitlesSelect').add(opt, null)
			hls.subtitleTrack  = document.getElementById('playerSubtitlesSelect').selectedIndex
			document.getElementById('trackselection').style.display = 'block';
		});
	} else {
		showStatusWindow('HLS playback is not supported by this browser')
	}

	return false;

}

async function playerAudioSelectOnchange(e) {
	hls.audioTrack  = document.getElementById('playerAudioSelect').selectedIndex
}

async function playerSubtitlesSelectOnchange(e) {
	hls.subtitleTrack  = document.getElementById('playerSubtitlesSelect').selectedIndex
}

async function updateStream(e) {
	e.preventDefault();

	n = getId(this.id)

	if (this.id.includes("speedup") && document.getElementById(this.id).checked && !document.getElementById('ondemand' + n).checked) {
		var result = confirm("Are you sure you want to set SpeedUp?\n\nSetting this option is only recommened to speed-up playback start when the OnDemand option is selected.");
		if (result != true) {
			document.getElementById(this.id).checked = false;
			return
		}
	}

	name = document.getElementById('name' + n).innerText

	vStreamId = $('#vStreamId' + n).val()
	aStreamId = ""
	$('#aStreamId' + n).val().forEach(function(i) {
		if (i == "all") {
			aStreamId = "all"
		} else if (i == "none") {
			aStreamId = "none"
		} else if (aStreamId != "all" && aStreamId != "none") {
			if (aStreamId.length != 0 ) {
				aStreamId += ","
			}
			aStreamId += i
		}
	});
	sStreamId = ""
	$('#sStreamId' + n).val().forEach(function(i) {
		if (i == "all") {
			sStreamId = "all"
		}
		if (sStreamId != "all") {
			if (sStreamId.length != 0 ) {
				sStreamId += ","
			}
			sStreamId += i
		}
	});

	prevParams = streamsPrevParams.get(n);
	if (!prevParams) {
		showStatusWindow('internal error');
	}

	prevKeys = ""
	if (prevParams.Keys) {
		for (let i = 0; i <  prevParams.Keys.length; i++) {
			prevKeys += prevParams.Keys[i]
			if ( i < prevParams.Keys.length-1) {
				prevKeys += "\n"
			}
		}
	}

	startTime = document.getElementById('rangeStartTime' + n).value
	endTime = document.getElementById('rangeEndTime' + n).value
	if (startTime == '') {
		startTime = "00:00"
	}
	if (endTime == '') {
		endTime = "00:00"
	}

	try {
		start = new Date(document.getElementById('rangeStartDate' + n).value + 'T' + startTime + ":00").toISOString()
	} catch (error) {
		start = ""
	}
	try {
		end = new Date(document.getElementById('rangeEndDate' + n).value + 'T' + endTime + ":00").toISOString()
	} catch (error) {
		end = ""
	}

	data = {
		Action : 'updatestream',
		ProviderId : getProviderId(this.id),
		Streams : [
			{
				Name : name,
				Id : getStreamId(this.id),
				ManifestUrl : document.getElementById('manifestUrl' + n).value,
				ManifestScript : document.getElementById('manifestscript' + n).value,
				Mode : document.getElementById('mode' + n).options[document.getElementById('mode' + n).selectedIndex].value,
				RunningMode : document.getElementById('runningmode' + n).options[document.getElementById('runningmode' + n).selectedIndex].value,
				OutputMode : document.getElementById('outputmode' + n).options[document.getElementById('outputmode' + n).selectedIndex].value,
				VStreamId: vStreamId,
				AStreamId: aStreamId,
				SStreamId: sStreamId,
				CdmType: document.getElementById('cdmtype' + n).options[document.getElementById('cdmtype' + n).selectedIndex].value,
				CdmMode: document.getElementById('cdmmode' + n).options[document.getElementById('cdmmode' + n).selectedIndex].value,
				Cdm: document.getElementById('cdm' + n).value,
				PRLAVersion: document.getElementById('prlaversion' + n).value,
				PRClientVersion: document.getElementById('prclientversion' + n).value,
				PRCustomData: document.getElementById('prcustomdata' + n).value,
				Keys: document.getElementById('keys' + n).value,
				OnDemand: document.getElementById('ondemand' + n).checked,
				Autostart: document.getElementById('autostart' + n).checked,
				PipeOutputParams: document.getElementById('pipeoutputparams' + n).value,
				ProtoOutputParams: document.getElementById('protooutputparams' + n).value,
				SessionManifest: document.getElementById('sessionmanifest' + n).checked,
				SpeedUp: document.getElementById('speedup' + n).checked,
				UseCdm: document.getElementById('usecdm' + n).checked,
				NetworkOverride: document.getElementById('network' + n).checked,
				ModeOverride: document.getElementById('modeoverride' + n).checked,
				RecordEvent: document.getElementById('recordevent' + n).checked,
				IgnoreUpdate: document.getElementById('ignoreupdate' + n).checked,
				FixIvSize: document.getElementById('fixivsize' + n).checked,
				TimeRange: document.getElementById('timerange' + n).checked,
				Bind: document.getElementById('bind' + n).value,
				Proxy: document.getElementById('proxy' + n).value,
				Doh: document.getElementById('doh' + n).value,
				NetworkScope:  $('#networkscope' + n).val().toString(),
				RangeStartTime: start,
				RangeEndTime: end
			}
		],

		PrevStreams : [
			{
				Name : name,
				Id : getStreamId(this.id),
				ManifestUrl: prevParams.Manifest,
				ManifestScript: prevParams.ManifestScript,
				Mode: prevParams.Mode,
				RunningMode: prevParams.RunningMode,
				OutputMode: prevParams.OutputMode,
				VStreamId: prevParams.Video,
				AStreamId: prevParams.Audio,
				SStreamId: prevParams.Subtitles,
				CdmType: prevParams.CdmType,
				CdmMode: prevParams.CdmMode,
				Cdm: prevParams.Cdm,
				PRLAVersion: prevParams.PRLAVersion,
				PRClientVersion: prevParams.PRClientVersion,
				PRCustomData: prevParams.PRCustomData,
				Keys: prevKeys,
				OnDemand : prevParams.OnDemand,
				Autostart: prevParams.Autostart,
				PipeOutputParams: prevParams.PipeOutputParams,
				ProtoOutputParams: prevParams.ProtoOutputParams,
				SessionManifest: prevParams.SessionManifest,
				SpeedUp: prevParams.SpeedUp,
				UseCdm: prevParams.UseCdm,
				NetworkOverride: prevParams.NetworkOverride,
				ModeOverride: prevParams.ModeOverride,
				RecordEvent: prevParams.RecordEvent,
				IgnoreUpdate: prevParams.IgnoreUpdate,
				FixIvSize: prevParams.FixIvSize,
				TimeRange: prevParams.TimeRange,
				Bind: prevParams.Bind,
				Proxy: prevParams.Proxy,
				Doh: prevParams.Doh,
				NetworkScope: prevParams.NetworkScope,
				UserAgent: prevParams.UserAgent,
				RangeStartTime: prevParams.RangeStartTime,
				RangeEndTime: prevParams.RangeEndTime
			}
		]	
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
		return
	} else {
		// Update prevparams
		prevParams.Manifest = data.Streams[0].ManifestUrl
		prevParams.ManifestScript = data.Streams[0].ManifestScript
		prevParams.Mode = data.Streams[0].Mode
		prevParams.RunningMode = data.Streams[0].RunningMode
		prevParams.OutputMode = data.Streams[0].OutputMode
		prevParams.Video = data.Streams[0].VStreamId
		prevParams.Audio = data.Streams[0].Audio
		prevParams.Subtitles = data.Streams[0].Subtitles
		prevParams.CdmType = data.Streams[0].CdmType
		prevParams.CdmMode = data.Streams[0].CdmMode
		prevParams.Cdm = data.Streams[0].Cdm
		prevParams.PRClientVersion = data.Streams[0].PRClientVersion
		prevParams.PRCustomData = data.Streams[0].PRCustomData
		prevParams.Keys = []
		if  (data.Streams[0].Keys) {
			data.Streams[0].Keys.split(",").forEach(function(id) {
				prevParams.Keys.push(id)
			});
		}
		prevParams.OnDemand = data.Streams[0].OnDemand 
		prevParams.Autostart = data.Streams[0].Autostart 
		prevParams.PipeOutputParams = data.Streams[0].PipeOutputParams
		prevParams.ProtoOutputParams = data.Streams[0].ProtoOutputParams
		prevParams.SessionManifest = data.Streams[0].SessionManifest 
		prevParams.SpeedUp = data.Streams[0].SpeedUp 
		prevParams.UseCdm = data.Streams[0].UseCdm 
		prevParams.NetworkOverride = data.Streams[0].NetworkOverride 
		prevParams.ModeOverride = data.Streams[0].ModeOverride 
		prevParams.RecordEvent = data.Streams[0].RecordEvent 
		prevParams.IgnoreUpdate = data.Streams[0].IgnoreUpdate 
		prevParams.FixIvSize = data.Streams[0].FixIvSize 
		prevParams.Bind = data.Streams[0].Bind
		prevParams.Proxy = data.Streams[0].Proxy
		prevParams.Doh = data.Streams[0].Doh
		prevParams.NetworkScope = data.Streams[0].NetworkScope
		prevParams.UserAgent = data.Streams[0].UserAgent
		prevParams.ManifestScriptParam = data.Streams[0].ManifestScriptParam
		prevParams.CdmParam = data.Streams[0].CdmParam

		streamsPrevParams.set(n, prevParams)
	}

	document.getElementById('saveall').removeAttribute("disabled");
	document.getElementById('saveprovider').removeAttribute("disabled");
};

// DELETE onCLick
async function massupdateClick(e) {
	e.preventDefault();

	if (document.getElementById('massupdateparams').options[document.getElementById('massupdateparams').selectedIndex].value == "") {
		return
	}

	if (!document.getElementById('massupdateselection').checked) {
		var result = confirm("Are you sure you want to set/unset " +
			document.getElementById('massupdateparams').options[document.getElementById('massupdateparams').selectedIndex].value +
			" for all streams?")
	        if (result != true) {
			return
	        }

		idsList = 'all'
	} else {
		idsList = getSelectedIds()
		if (idsList == "") {
			return;
		}
	}

	data = {
		Action : 'massupdate',
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		MassUpdateStreamIds: idsList,
		MassUpdateParam: document.getElementById('massupdateparams').options[document.getElementById('massupdateparams').selectedIndex].value,
		MassUpdateValue: (this.id == 'massupdateset')?"set":"unset"
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
		return
	} else {
		document.getElementById('saveall').removeAttribute("disabled");
		document.getElementById('saveprovider').removeAttribute("disabled");
	}
}

function collectUsersFromForm() {
	var users = [];
	var n = 0;
	while (document.getElementById('username' + n)) {
		var picked = $('#userProviders' + n).val();
		users.push({
			Username: document.getElementById('username' + n).innerText.trim(),
			Password: $('#password' + n).val(),
			Network: $('#network' + n).val(),
			IsAdmin: document.getElementById('isadmin' + n).checked,
			HasWebAccess: document.getElementById('haswebaccess' + n).checked,
			ProviderIds: picked ? picked : []
		});
		n++;
	}
	return users;
}

async function saveUsersClick(e) {
	e.preventDefault();
	var noauthEl = document.getElementById('noauth');
	var data = {
		Action: 'saveusers',
		NoAuth: !!(noauthEl && noauthEl.checked),
		Users: collectUsersFromForm()
	};
	let response = await fetch(document.getElementById('baseUrl').value + 'api/action', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		credentials: 'same-origin',
		body: JSON.stringify(data),
	});
	if (!response.ok) {
		showStatusWindow(await response.text());
		return;
	}
	showStatusWindow('Users saved');
	location.reload();
}


async function updateGlobal(e) {
	e.preventDefault();

	if (this.id == 'networkscope' || this.id == 'proxy' || this.id == 'bind' || this.id == 'doh') {
		document.getElementById('applyglobalnetwork').removeAttribute("disabled");
	}

	headers = []
	document.getElementById('headers').value.split("\n").forEach(function(h) {
		headers.push(h)
	});

	data = {
		Action : 'updateglobal',
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		Script: document.getElementById('script').value,
		Proxy: document.getElementById('proxy').value,
		Bind: document.getElementById('bind').value,
		Doh: document.getElementById('doh').value,
		NetworkScope: $('#networkscope').val().toString(),
		UserAgent: document.getElementById('useragent').value,
		XForwardedFor: document.getElementById('xforwardedfor').value,
		EventsAutorefresh: document.getElementById('eventsautorefresh').checked,
		EventsAutoremove: document.getElementById('eventsautoremove').checked,
		ChannelsAutoremove: document.getElementById('channelsautoremove').checked,
		MaxConcurrentStreams: parseInt(document.getElementById('maxconcurrentstreams').value, 10),
		RunningMode: document.getElementById('runningmode').value,
		OutputMode: document.getElementById('outputmode').value,
		AlwaysResetSession: document.getElementById('alwaysresetsession').checked,
		NoRestartOnError: document.getElementById('norestartonerror').checked,
		RestartFinished: document.getElementById('restartfinished').checked,
		NoRestartOnTrackChange: document.getElementById('norestartontrackchange').checked,
		ReuseEventIndex	: document.getElementById('reuseeventindex').checked,
		RestartDelay: parseInt(document.getElementById('restartdelay').value, 10),
		NbAnnouncedFragments: parseInt(document.getElementById('nbannouncedfragments').value, 10),
		HlsFragmentsDuration: parseInt(document.getElementById('hlsfragmentsduration').value, 10),
		PlaylistDuration: parseInt(document.getElementById('playlistduraion').value, 10),
		AutorestartPeriod: parseInt(document.getElementById('autorestartperiod').value, 10),
		PlaybackDelay: parseInt(document.getElementById('playbackdelay').value, 10),
		RandomAutostartPeriod: parseInt(document.getElementById('randomautostartperiod').value, 10),
		SequencialAutostartPeriod: parseInt(document.getElementById('sequencialautostartperdiod').value, 10),
		EpgTimezone: document.getElementById('epgtimezone').value,
		EventsRefreshPeriod: parseInt(document.getElementById('eventsrefreshperiod').value, 10),
		NoWaitFullPlaylist: document.getElementById('nowaitfullplaylist').checked,
		ContinuousPlayback: document.getElementById('continuousplayback').checked,
		IgnoreStaticDash: document.getElementById('ignorestaticdash').checked,
		UseDashDelay: document.getElementById('usedashdelay').checked,
		HttpGetRetries: parseInt(document.getElementById('httpgetretries').value, 10),
		StallDetectTimeout: parseInt(document.getElementById('stalldetecttimeout').value, 10),
		VmxUniqueId: document.getElementById('vmxuniqueid').value,
		Headers: headers
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	});

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
		return
	}

	document.getElementById('saveall').removeAttribute("disabled");
	document.getElementById('saveprovider').removeAttribute("disabled");
};

// START/STOP stream
async function startStopClick(e, id) {

	e.preventDefault();

        if (!id) {
                id = this.id
	}
		
	action = this.name

	if (id == 'startall' || id == 'stopall') {
		idsList = 'all'
		var result = confirm("Are you sure you want to " + action + " all streams?");
		if (result != true) {
			return
		}
		providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value;
	} else if (id == 'startautostart') {
		idsList = 'autostart'
		var result = confirm("Are you sure you want to " + action + " all autostart streams?");
		if (result != true) {
			return
		}
		providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value;
	} else if (id == 'startpage' || this.id == 'stoppage') {
		idsList = getPageIds()
		if (idsList == "") {
			return;
		}
		var result = confirm("Are you sure you want to " + action + " all streams from page?");
		if (result != true) {
			return
		}
		providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value;
	} else {
		n = getId(id)
		idsList = getStreamId(id)
		if (document.getElementById('startStop' + n).innerHTML.includes('play.svg')) {
			action = 'start'
		} else {
			action = 'stop'
		}
		providerId = getProviderId(id)
	}

	data = {
		Action : action,
		ProviderId : providerId,
		Streams : [
			{
				Id : idsList,
			}
		]	
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	}
};

// Shutdowx onCLicx
async function shutdownClick(e) {
	e.preventDefault();

	var result = confirm("Are you sure you want to shutdown drmsonic?");
        if (result != true) {
                return
        }

	data = {
		Action : "shutdown",
	};

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	});

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	}
};

async function saveClick(e) {
	e.preventDefault();

	if (this.id == 'saveall') {
		providerId = ""
	} else {
		providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value
	}

	data = {
		Action : "save",
		ProviderId : providerId
	};

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
		},
		credentials: 'same-origin',
		body: JSON.stringify(data),
	});

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		if (this.id == 'saveall') {
			document.getElementById('saveall').setAttribute("disabled","disabled");
		} else {
			document.getElementById('saveprovider').setAttribute("disabled","disabled");
		}
	}
};

async function rescanProvidersClick(e) {
	e.preventDefault();
	data = {
		Action : "rescanproviders"
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
	} else {
		showStatusWindow(text);
		if (text != "No new provider detected") {
			location.reload();
		}
	}
};


async function addProviderFormSubmit(e) {
	e.preventDefault();
	data = {
		Action : "addprovider",
		ProviderName : document.getElementById('addProviderText').value,
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
	} else {
		window.location.href = "/config/" + text
	}
};

// ADD onCLick
async function addStreamFormSubmit(e) {
	e.preventDefault();
	data = {
		Action : "addchannel",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		Streams: [
			{
				Name : document.getElementById('addText').value,
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		window.location.href = "/config/" +  document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value + "?search=" + document.getElementById('addText').value
	}
};

async function renameStreamClick(e) {
	e.preventDefault();

	n = getId(this.id)
	streamId = getStreamId(this.id)

	let text;
	let newName = prompt("Please enter new stream name", document.getElementById('name' + n).innerText);
	if (newName == null || newName == "") {
		return;
	}

	data = {
		Action : "renamechannel",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		Streams: [
			{
				Id : streamId,
				Name : newName
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		location.reload()
	}
};

async function renameProviderClick(e) {
	e.preventDefault();

	let text;
	let newName = prompt("Please enter new provider name", document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].innerText);
	if (newName == null || newName == "") {
		return;
	}

	data = {
		Action : "renameprovider",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		ProviderName: newName
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
	} else {
		window.location.href = "/config/" + text
	}
};



async function refreshConfigWithProvider(e) {
	e.preventDefault();
	window.location.href = "/config/" + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value
}

async function refreshStatusWithProvider(e) {
	e.preventDefault();
	window.location.href = "/status/" + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value
	document.getElementById('currentFilter').innerText = "";
}

async function refreshEpgWithProvider(e) {
	e.preventDefault();
	window.location.href = "/events/" + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value
}

async function eventsRefreshFormSubmit(e) {
	e.preventDefault();
	data = {
		Action : "eventsrefreshrequest",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
	}

	displayLoading(eventsloader);
	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	hideLoading(eventsloader);
	if (!response.ok)  {
		showStatusWindow(text);
		return
	} else {
		if (text == "No change in events") {
			bootbox.alert('No change in events');
		} else {
			bootbox.confirm({
				message: '<pre>' + text + '</pre>',
				callback: applyEvents
			});
		}
	}
}

async function applyEvents(result) {
	if (!result) {
		return
	}
	data = {
		Action : "eventsrefreshapply",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		bootbox.alert(response.statusText);
	} else {
		location.reload();
	}
};

async function channelsUpdateFormSubmit(e) {
	e.preventDefault();
	data = {
		Action : "channelsupdaterequest",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
	}

	displayLoading(channelsloader);
	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	hideLoading(channelsloader);
	if (!response.ok) {
		showStatusWindow(text);
	} else {
		if (text == "No change in channels") {
			bootbox.alert('No change in channels');
		} else {
			bootbox.confirm({
				message: '<pre>' + text + '</pre>',
				callback: applyChannels
			});
		}
	}

};

async function applyChannels(result) {
	if (!result) {
		return
	}
  	data = {
  		Action : "channelsupdateapply",
  		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
  	}

  	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
  		method: 'POST', // or 'PUT'
  		headers: {
  			'Content-Type': 'application/json',
  		},
  		body: JSON.stringify(data),
  	})

  	if (!response.ok) {
  		text = await response.text();
  		bootbox.alert(response.statusText);
  	} else {
  		location.reload();
  	}
}

async function getEvents(epgtable) {

	data = {
		Action : "getevents",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		document.getElementById('eventsstatus').text = text;
		return
	} else {
		document.getElementById('eventsstatus').text = '';

		data = JSON.parse(text);
		if (!data.Entries) {
			return
		}
	}

	epgtable.rows().remove()

	providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value

	data.Entries.forEach(function(entry){
		if (entry.OnAirId != '') {
			color = "green"
		} else {
			color = "white"
		}

		evid = '<a id="name_' + providerId + '_' + entry.EventId + '" href="' + entry.StreamingUrl + '" onclick="return setPlayer(event, id);">' + entry.EventId + '</a>'
		if (entry.OnAirId != '') {
			evid += '<br><a id="name_' + providerId + '_' + entry.OnAirId + '" href="' + entry.StreamingOnAirUrl + '" onclick="return setPlayer(event, id);">' + entry.OnAirId + '</a>'
		}
		conf = '<a href="config/' + providerId + '?search=' + entry.Title + '"><img style="width: 20px; height: auto;" src="static/gear.svg" /></a>'

		if (entry.Running) {
			play = '<button onclick="return startStopClick(event, id)"; style="border-color: transparent;" type="button" class="btn" id="startStop_' + providerId + '_' + entry.EventId + '"><img id="playstop_' + providerId + '_' + entry.EventId + '" style="width: 20px; height: auto;" src="static/stop.svg"/></button>'
		} else {
			play = '<button onclick="return startStopClick(event, id)"; style="border-color: transparent;" type="button" class="btn" id="startStop_' + providerId + '_' + entry.EventId + '"><img id="playstop_' + providerId + '_' + entry.EventId + '" style="width: 20px; height: auto;" src="static/play.svg"/></button>'
		}

		if (entry.Status.includes("Error:") || entry.Status.includes('Key(s) missing')) {
			status = entry.Status.substring(0, 20).substring(0, entry.Status.indexOf('\n'));
			if (entry.Status.length > 20  || entry.Status.includes('\n')) {
				if (!entry.Status.includes('autorestart in')) {
					status += "..."
				}
			}
		} else {
			status = entry.Status;
		}
		statusColor = entry.StatusColor;

		epgtable.row.add(['<a id="title_' + providerId + '_' + entry.EventId + '" style="color: ' + color + '";>' +  entry.Title + '</a>', '<pre id="status_' + providerId + '_' + entry.EventId + '" onclick="return showErrorLogWindow(event, id)"; style="color: ' + statusColor + ';">' + status + '</pre><div hidden id="statusfull_' + providerId + '_' + entry.EventId + '">' + entry.Status + '</div>', entry.StartFmt, entry.EndFmt, evid, play+conf]).draw()
	})

	$(epgtable.column(2).header()).text('Start Date (' + data.Timezone + ')');
	$(epgtable.column(3).header()).text('End Date (' + data.Timezone + ')');


};

async function updateEvents(epgtable) {

	if (document.hidden) {
		return
	}

	data = {
		Action : "getevents",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		document.getElementById('eventsstatus').text = text;
		return
	} else {
		document.getElementById('eventsstatus').text = '';

		data = JSON.parse(text);
		if (!data.Entries) {
			return
		}
	}

	providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value

	data.Entries.forEach(function(entry){
		if (document.getElementById('name_' + providerId + '_' + entry.EventId) && isInViewport(document.getElementById('name_' + providerId + '_' + entry.EventId))) {
			if (entry.Running) {
				document.getElementById('playstop_' + providerId + '_' + entry.EventId).src = "static/stop.svg"
			} else {
				document.getElementById('playstop_' + providerId + '_' + entry.EventId).src = "static/play.svg"
			}

			if (entry.OnAirId != '') {
				document.getElementById('title_' + providerId + '_' + entry.EventId).style.color = "green"
			} else {
				document.getElementById('title_' + providerId + '_' + entry.EventId).style.color = "white"
			}


			if (entry.Status.includes("Error:") || entry.Status.includes('Key(s) missing')) {
				status = entry.Status.substring(0, 20).substring(0, entry.Status.indexOf('\n'));
				if (entry.Status.length > 20  || entry.Status.includes('\n')) {
					if (!entry.Status.includes('autorestart in')) {
						status += "..."
					}
				}
			} else {
				status = entry.Status;
			}
			document.getElementById('status_' + providerId + '_' + entry.EventId).innerText = status;
			document.getElementById('status_' + providerId + '_' + entry.EventId).style.color = entry.StatusColor;
		}
	})


};

async function getReplays(replaystable) {

	data = {
		Action : "getreplays",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		document.getElementById('eventsstatus').text = text;
		return
	} else {
		document.getElementById('eventsstatus').text = '';

		data = JSON.parse(text);
		if (!data.Replays) {
			return
		}
	}

	replaystable.rows().remove()

	data.Replays.forEach(function(replay){
		if (replay.OnAir) {
			color = "green"
		} else {
			color = "white"
		}

		replayid = '<a id="replay_' + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value + '_' + replay.EventId + '" href="' + replay.StreamingUrl + '" onclick="return setPlayer(event, id);">' + replay.EventId + '</a>';
		del = '<input type="button" class="btn btn-primary" id="deletereplay_' + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value + '_' + replay.EventId + '" value="DELETE" onclick="return deleteReplayClick(event, id);"/>'

		replaystable.row.add(['<a style="color: ' + color + '";>' + replay.EventName + '</a>', replay.StartFmt, replay.EndFmt, replayid, del]).draw()
	})
	
	$(replaystable.column(1).header()).text('Start Date (' + data.Timezone + ')');
	$(replaystable.column(2).header()).text('End Date (' + data.Timezone + ')');

};

async function deleteReplayClick(e, id) {
	e.preventDefault();
	n = id.replace("deletereplay", "")

	var result = confirm("Are you sure you want to delete replay?");
	if (result != true) {
		return
	}

	data = {
		Action : "deletereplay",
		ProviderId : getProviderId(n),
		Streams: [
			{
				Id : getStreamId(n),
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		location.reload();
	}
};


async function applyGlobalNetworkClick(e) {
	e.preventDefault();

	data = {
		Action : "applyglobalnetwork",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
	} else {
		document.getElementById('applyglobalnetwork').setAttribute("disabled","disabled");
	}

};


// ADD onCLick
async function addUserFormSubmit(e) {
	e.preventDefault();
	data = {
		Action : "adduser",
		Users : [
			{
				Username : document.getElementById('addUserText').value,
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
		},
		credentials: 'same-origin',
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		location.reload();
	}
};

async function deleteSelectionClick(e) {
	e.preventDefault();

	idsList = getSelectedIds()
	if (idsList == "") {
		return;
	}

	var result = confirm("Are you sure you want to delete:\n\n" +  idsList);
        if (result != true) {
                return
        }

	data = {
		Action : "deletestream",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		Streams: [
			{
				Id : idsList
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		location.reload();
	}
};

async function deleteProviderClick(e) {
	e.preventDefault();

	var result = confirm("Are you sure you want to delete provider " + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].innerText + "?\n\nWarning: provider's config will really be deleted after your press [SAVE PROVIDERS]");
        if (result != true) {
                return
        }

	data = {
		Action : "deleteprovider",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		window.location.href = "/config";
	}
};


// EXPORT onCLick
async function exportProviderClick(e) {
	e.preventDefault();

	var idsList = ""
	if (this.id == 'exportselection') {
		idsList = getSelectedIds()
		if (idsList == "") {
			return
		}
	} else {
		idsList = "all"
	}

	data = {
		Action : "export",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		Streams: [
			{
				Id : idsList
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
	} else {
		var element = document.createElement('a');
		element.setAttribute('href', 'data:text/plain;charset=utf-8,' + encodeURIComponent(text));
		element.setAttribute('download', document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value + ".cfg");
		element.style.display = 'none';
		document.body.appendChild(element);
		element.click();
		document.body.removeChild(element);
	}
};

async function importProviderClick(e) {
	e.preventDefault();
	var el = window._protected_reference = document.createElement("INPUT");
	el.type = "file";
  
	el.addEventListener('change', async function(ev2) {
		if (el.files.length) {
			var reader = new FileReader();
			reader.readAsText(el.files[0], "UTF-8")
			reader.onload = async function (evt) {
				data = {
					Action : "import",
					ProviderName : document.getElementById('addProviderText').value,
					ProviderImportConfig: evt.target.result
				}

				let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
					},
					body: JSON.stringify(data),
				})

				if (!response.ok) {
					text = await response.text();
					showStatusWindow(text);
				} else {
					location.reload();
				}
			}
		}
	});

	el.click();
}

function usersTableClick(e) {
	var btn = e.target.closest('[data-user-action]');
	if (!btn) {
		return;
	}
	e.preventDefault();
	var n = btn.getAttribute('data-user-index');
	if (btn.getAttribute('data-user-action') == 'delete') {
		deleteUser(n);
	} else {
		renameUser(n);
	}
}

async function deleteUser(n) {
	var result = confirm("Are you sure you want to delete " + document.getElementById('username' + n).innerText + "?");
	if (result != true) {
		return
	}

	data = {
		Action : "deleteuser",
		Users : [
			{
				Username : document.getElementById('username' + n).innerText,
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
		},
		credentials: 'same-origin',
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		location.reload();
	}
};

async function renameUser(n) {
	let newName = prompt("Please enter new user name");
	if (newName == null || newName == "") {
		return;
	}

	data = {
		Action : "renameuser",
		Users : [
			{
				Username : document.getElementById('username' + n).innerText,
			}
		],
		NewUserName: newName
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
		},
		credentials: 'same-origin',
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		showStatusWindow(await response.text());
	} else {
		document.getElementById('username' + n).innerText = newName;
	}
};

// REFRESH  onCLick
async function refreshClick(e) {
	e.preventDefault();

	if (this.id == 'refreshall') {
		var result = confirm("Are you sure you want to refresh all streams?");
		if (result != true) {
			return
		}
		idsList = 'all'
		providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value
		action = this.name
	} else if (this.id == 'refreshpage') {
		var result = confirm("Are you sure you want to refresh all streams from current page?");
		if (result != true) {
			return
		}
		idsList = getPageIds()
		if (idsList == "") {
			return;
		}
	} else {
		n = getId(this.id)
		idsList = getStreamId(this.id)
	}

	data = {
		Action : "refresh",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		Streams: [
			{
				Id : idsList,
			},
		]
	}

	displayLoading(globalloader);
	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	hideLoading(globalloader);
	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		document.getElementById('saveall').removeAttribute("disabled");
		document.getElementById('saveprovider').removeAttribute("disabled");

	}

};

async function refreshKeysClick(e) {
	e.preventDefault();

	n = getId(this.id)
	idsList = getStreamId(this.id)

	data = {
		Action : "refreshkeys",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		Streams: [
			{
				Id : idsList,
			},
		]
	}

	displayLoading(globalloader);
	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	hideLoading(globalloader);
	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		document.getElementById('saveall').removeAttribute("disabled");
		document.getElementById('saveprovider').removeAttribute("disabled");

	}

};

async function flushKeysClick(e) {
	e.preventDefault();

	n = getId(this.id)
	idsList = getStreamId(this.id)

	var result = confirm("Are you sure you want to flush keys from \n\n" +  idsList);
        if (result != true) {
                return
        }

	data = {
		Action : "flushkeys",
		ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
		Streams: [
			{
				Id : idsList,
			},
		]
	}

	displayLoading(globalloader);
	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	hideLoading(globalloader);
	if (!response.ok) {
		text = await response.text();
		showStatusWindow(text);
	} else {
		document.getElementById('saveall').removeAttribute("disabled");
		document.getElementById('saveprovider').removeAttribute("disabled");

	}

};


async function populateStream(item) {

	id = '_' + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value + '_' + item.Id

	// Store value
	streamsPrevParams.set(id, item.Stream)

	nameId = 'name' + id
	if (!document.getElementById(nameId)) {
		return
	}

	// Mode
	mode = 'mode' + id
	for (i=0; i<document.getElementById(mode).length; i++) {
		if (document.getElementById(mode).options[i].value == item.Stream.Mode) {
			document.getElementById(mode).selectedIndex = i;
			break
		}
	}
	runningmode = 'runningmode' + id
	for (i=0; i<document.getElementById(runningmode).length; i++) {
		if (document.getElementById(runningmode).options[i].value == item.Stream.RunningMode) {
			document.getElementById(runningmode).selectedIndex = i;
			break
		}
	}
	outputmode = 'outputmode' + id
	for (i=0; i<document.getElementById(outputmode).length; i++) {
		if (document.getElementById(outputmode).options[i].value == item.Stream.OutputMode) {
			document.getElementById(outputmode).selectedIndex = i;
			break
		}
	}

	// MPD
	document.getElementById('headersDiv' + id).style.display = 'none';
	document.getElementById('showheaders' + id).style.display = 'none';

	document.getElementById('headersmanifest' + id).value = ""
	document.getElementById('headersmedia' + id).value = ""
	var streamHeaders = item.Stream.Headers;
	if (streamHeaders && !Array.isArray(streamHeaders)) {
		var hasManifestHeaders = streamHeaders.Manifest && Object.keys(streamHeaders.Manifest).length > 0;
		var hasMediaHeaders = streamHeaders.Media && Object.keys(streamHeaders.Media).length > 0;
		if (hasManifestHeaders) {
			Object.keys(streamHeaders.Manifest).forEach(key => {
				document.getElementById('headersmanifest' + id).value += key + ': ' + streamHeaders.Manifest[key] + '\n';
			});
		}
		if (hasMediaHeaders) {
			Object.keys(streamHeaders.Media).forEach(key => {
				document.getElementById('headersmedia' + id).value += key + ': ' + streamHeaders.Media[key] + '\n';
			});
		}
		if (hasManifestHeaders || hasMediaHeaders) {
			document.getElementById('showheaders' + id).style.display = 'block';
			document.getElementById('headersDiv' + id).style.marginLeft = 'auto';
			document.getElementById('headersDiv' + id).style.marginRight = 'auto';
		}
	}

	document.getElementById('manifestscript' + id).value = item.Stream.ManifestScript || "";
	document.getElementById('manifestUrl' + id).value = item.Stream.Manifest || "";

	// Network
	document.getElementById('proxy' + id).value = item.Stream.Proxy || "";
	document.getElementById('bind' + id).value = item.Stream.Bind || "";
	document.getElementById('doh' + id).value = item.Stream.Doh || "";
	$('#networkscope' + id).val((item.Stream.NetworkScope || "").split(","));
	$('#networkscope' + id).selectpicker("refresh");

	// Video
	var vStreamId = 'vStreamId' + id
	$('#' + vStreamId).empty();

	var videoSel = item.Stream.Video || "best";
	sel = (videoSel == "best" || videoSel == "") ? "selected" : "";
	$('#' + vStreamId).append('<option ' + sel + ' value="best">Best quality</option>');
	sel = (videoSel == "all") ? "selected" : "";
	$('#' + vStreamId).append('<option ' + sel + ' value="all">All qualities</option>');

	if (item.Stream.VideoList != null) {
		for (i=0; i<item.Stream.VideoList.length; i++) {
			if (item.Stream.VideoList[i].Id == "best" || item.Stream.VideoList[i].Id == "all") {
				continue
			}
			sel = ""
			if (item.Stream.VideoList[i].Id == videoSel) {
				sel = "selected"
			}

			dataSubtext = ""
			if (item.Stream.VideoList[i].Kid == "") {
				dataSubtext = "&#63; No KID available"
			} else {
				dataSubtext = "&#10060; Key not found (kid: " + item.Stream.VideoList[i].Kid + ")"
				if (item.Stream.Keys != null) {
					for (j=0; j<item.Stream.Keys.length; j++) {
						if (item.Stream.Keys[j].startsWith(item.Stream.VideoList[i].Kid + ":")) {
							 dataSubtext = "&#9989; Key found (kid: " + item.Stream.VideoList[i].Kid + ")"
						}
					}
				}
			}

			$('#' + vStreamId).append('<option ' + sel + ' data-subtext="' + dataSubtext + '" value="'+item.Stream.VideoList[i].Id+'">'+item.Stream.VideoList[i].Desc+'</option>');
		}
	}
	$('#' + vStreamId).selectpicker("refresh");

	// Audio
	var aStreamId = 'aStreamId' + id
	$('#' + aStreamId).empty();
	if (item.Stream.Audio == "all") {
		sel = "selected"
	} else {
		sel = ""
	}
	$('#' + aStreamId).append('<option ' + sel + ' value="all">All</option>');
	if (item.Stream.Audio == "none") {
		sel = "selected"
	} else {
		sel = ""
	}
	$('#' + aStreamId).append('<option ' + sel + ' value="none">Disabled</option>');
	if (item.Stream.AudioList != null) {
		for (i=0; i<item.Stream.AudioList.length; i++) {
			sel = ""
			item.Stream.Audio.split(',').forEach(function(id) {
				if (item.Stream.AudioList[i].Id == id || id == "all") {
					sel = "selected"
				}
			});

			dataSubtext = ""
			if (item.Stream.AudioList[i].Kid == "") {
				dataSubtext = "&#63; No KID available"
			} else {
				dataSubtext = "&#10060; Key not found (kid: " + item.Stream.AudioList[i].Kid + ")"
				if (item.Stream.Keys != null) {
					for (j=0; j<item.Stream.Keys.length; j++) {
						if (item.Stream.Keys[j].startsWith(item.Stream.AudioList[i].Kid + ":")) {
							 dataSubtext = "&#9989; Key found (kid: " + item.Stream.AudioList[i].Kid + ")"
						}
					}
				}
			}

			$('#' + aStreamId).append('<option ' + sel + ' data-subtext="' + dataSubtext + '" value="'+item.Stream.AudioList[i].Id+'">'+item.Stream.AudioList[i].Desc+'</option>');
		}
	}
	$('#' + aStreamId).selectpicker("refresh");

	// Subtitles
	var sStreamId = 'sStreamId' + id
	$('#' + sStreamId).empty();
	if (item.Stream.Subtitles == "all") {
		sel = "selected"
	} else {
		sel = ""
	}
	$('#' + sStreamId).append('<option ' + sel + ' value="all">All</option>');
	if (item.Stream.SubtitlesList != null) {
		for (i=0; i<item.Stream.SubtitlesList.length; i++) {
			sel = ""
			item.Stream.Subtitles.split(',').forEach(function(id) {
				if (item.Stream.SubtitlesList[i].Id == id || id == "all") {
					sel = "selected"
				}
			});

			$('#' + sStreamId).append('<option ' + sel + ' value="'+item.Stream.SubtitlesList[i].Id+'">'+item.Stream.SubtitlesList[i].Desc+'</option>');
		}
	}
	$('#' + sStreamId).selectpicker("refresh");

	vStreamIdDiv = 'vStreamIdDiv' + id
	aStreamIdDiv = 'aStreamIdDiv' + id
	sStreamIdDiv = 'sStreamIdDiv' + id
	if ( item.Stream.VideoList != null && item.Stream.VideoList.length > 0 ) {
		document.getElementById(vStreamIdDiv).style.display = 'block';
	} else {
		document.getElementById(vStreamIdDiv).style.display = 'none';
	}
	if ( item.Stream.AudioList != null && item.Stream.AudioList.length > 0 ) {
		document.getElementById(aStreamIdDiv).style.display = 'block';
	} else {
		document.getElementById(aStreamIdDiv).style.display = 'none';
	}
	if ( item.Stream.SubtitlesList != null && item.Stream.SubtitlesList.length > 0 ) {
		document.getElementById(sStreamIdDiv).style.display = 'block';
	} else {
		document.getElementById(sStreamIdDiv).style.display = 'none';
	}

	startDate = 'rangeStartDate' + id
	startTime = 'rangeStartTime' + id
	endDate = 'rangeEndDate' + id
	endTime = 'rangeEndTime' + id

	if (item.Stream.RangeStartTime) {
		localStart = new Date(item.Stream.RangeStartTime);
		document.getElementById(startDate).value = localStart.toLocaleString('fr-CA').substr(0, 10).replaceAll('/',':');
		document.getElementById(startTime).value = localStart.toLocaleString('fr-FR').substr(11,5);
	}

	if (item.Stream.RangeEndTime) {
		localEnd = new Date(item.Stream.RangeEndTime);
		document.getElementById(endDate).value = localEnd.toLocaleString('fr-CA').substr(0, 10).replaceAll('/',':');
		document.getElementById(endTime).value = localEnd.toLocaleString('fr-FR').substr(11,5);
	}


	// CDM Type
	cdmtype = 'cdmtype' + id
	for (i=0; i<document.getElementById(cdmtype).length; i++) {
		if (document.getElementById(cdmtype).options[i].value == item.Stream.CdmType) {
			document.getElementById(cdmtype).selectedIndex = i;
			break
		}
	}


	// CDM Mode
	cdmmode = 'cdmmode' + id
	if (item.Stream.CdmMode == "") {
		item.Stream.CdmMode = "internal"
	}
	for (i=0; i<document.getElementById(cdmmode).length; i++) {
		if (document.getElementById(cdmmode).options[i].value == item.Stream.CdmMode) {
			document.getElementById(cdmmode).selectedIndex = i;
			break
		}
	}

	// Cdm
	cdm = 'cdm' + id
	document.getElementById(cdm).value = item.Stream.Cdm

	// LA and Client version and custom data
	prlaversion = 'prlaversion' + id
	document.getElementById(prlaversion).value = item.Stream.PRLAVersion
	prclientversion = 'prclientversion' + id
	document.getElementById(prclientversion).value = item.Stream.PRClientVersion
	prcustomdata = 'prcustomdata' + id
	document.getElementById(prcustomdata).value = item.Stream.PRCustomData

	// Keys
	keys = 'keys' + id
	document.getElementById(keys).value = ""
	if (item.Stream.Keys != null) {
		for (i=0; i<item.Stream.Keys.length; i++) {
			document.getElementById(keys).value += item.Stream.Keys[i] + '\n';
		}
	}

	// Checkboxes
	var i = 0;
	document.getElementById('autostart' + id).checked = item.Stream.Autostart;
	document.getElementById('ondemand' + id).checked = item.Stream.OnDemand;
	document.getElementById('sessionmanifest'+ id).checked = item.Stream.SessionManifest;
	document.getElementById('speedup' + id).checked = item.Stream.SpeedUp;
	document.getElementById('usecdm' + id).checked = item.Stream.UseCdm;
	document.getElementById('network' + id).checked = item.Stream.NetworkOverride;
	document.getElementById('modeoverride' + id).checked = item.Stream.ModeOverride;
	document.getElementById('recordevent' + id).checked = item.Stream.RecordEvent;
	document.getElementById('ignoreupdate' + id).checked = item.Stream.IgnoreUpdate;
	document.getElementById('fixivsize' + id).checked = item.Stream.FixIvSize;
	document.getElementById('timerange' + id).checked = item.Stream.TimeRange;
	document.getElementById('pipeoutputparams' + id).value = item.Stream.PipeOutputParams;
	document.getElementById('protooutputparams' + id).value = item.Stream.ProtoOutputParams;

	if (item.Stream.UseCdm) {
		document.getElementById('cdmdiv' + id).style.display = 'block';
		document.getElementById('keys' + id).rows = "3";
	} else {
		document.getElementById('cdmdiv' + id).style.display = 'none';
		document.getElementById('keys' + id).rows = "5";
	}

	if (item.Stream.SessionManifest) {
		document.getElementById('manifestscript' + id).style.display = 'block';
	} else {
		document.getElementById('manifestscript' + id).style.display = 'none';
	}
}

function ImageExist(url)  {
	var img = new Image();
	img.src = url;
	return img.height != 0;
}

function isInViewport(element) {
	    const rect = element.getBoundingClientRect();
	    return (
		            rect.top >= 0 &&
		            rect.left >= 0 &&
		            rect.bottom <= (window.innerHeight || document.documentElement.clientHeight) &&
		            rect.right <= (window.innerWidth || document.documentElement.clientWidth)
		        );
}

async function checkallChange(e) {
	e.preventDefault();
	var rowArray = table.rows({ page: 'current' }).data().toArray();
	if (rowArray == null) {
		return ""
	}

	rowArray.forEach( function (row) {
		id = /name(_[^_]*_[^"]*)/.exec(row[2])[1]
		document.getElementById('checked' + id).checked = document.getElementById('checkall').checked;
	});
}

var updateStreamsBusy = false;
var updateStreamsPending = false;
var updateStreamsInit = false;
var streamsRendered = false;

async function updateStreams(table, init) {
	if (document.hidden) {
		return
	}
	var sel = document.getElementById('providerSelection');
	if (!sel || sel.selectedIndex < 0 || !sel.options.length) {
		return
	}
	if (!streamsRendered) {
		init = 1;
	}
	if (updateStreamsBusy) {
		updateStreamsPending = true;
		updateStreamsInit = updateStreamsInit || !!init;
		return
	}
	updateStreamsBusy = true;
	try {
		await updateStreamsBody(table, init, sel.options[sel.selectedIndex].value);
		if (init) {
			streamsRendered = true;
		}
	} catch (err) {
		console.error(err);
	} finally {
		updateStreamsBusy = false;
		if (updateStreamsPending) {
			var pending = updateStreamsInit ? 1 : 0;
			updateStreamsPending = false;
			updateStreamsInit = false;
			updateStreams(table, pending);
		}
	}
}

async function updateStreamsBody(table, init, providerId) {
	idsList = getVisibleIds(init);

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
		},
		credentials: 'same-origin',
		body: JSON.stringify({
					Action : "getstatus",
					ProviderId : providerId,
					Streams: [  { "Id" : idsList, }, ]
					}),
	}).catch(error => { showStatusWindow('remote server unreachable'); clearInterval(intervalId); intervalId = 0; return null;});

	if (intervalId == 0 || !response) {
		return
	}

	let data = "";

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
		return
	}

	data = JSON.parse(text);

	if (document.getElementById('filterselection') && document.getElementById('filterselection').selectedIndex > 0 && (data.StreamsCount != table.rows().count())) {
		if (data.Providers && data.Providers[0] && data.Providers[0].Id != providerId) {
			return
		}
		clearInterval(intervalId);
		location.reload();
		return
	}

	// Build list of channels to update
	var idsList = ""
	if (data.Providers) {
		data.Providers.forEach(function(provider){
			if (provider.Streams) {
				provider.Streams.forEach(function(stream){
					if (PChangeCounter[stream.Id] == undefined) {
						PChangeCounter[stream.Id] = -1;
					}
		
					if (init || stream.PChangeCounter > PChangeCounter[stream.Id] ) {
						idsList += stream.Id + ","
					}

					PChangeCounter[stream.Id] = stream.PChangeCounter
				});
			}
		});
	}
	idsList = idsList.slice(0, -1)

	if (GlobalChangeCounter == undefined) {
		GlobalChangeCounter = -1;
	}

	if (idsList != '' || data.GlobalChangeCounter > GlobalChangeCounter) {
		let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
			},
			credentials: 'same-origin',
			body: JSON.stringify({
				Action : "getconf",
				ProviderId : providerId,
				Streams: [  { "Id" : idsList, } ]
			}),
		})

		text = await response.text();
		if (!response.ok) {
			showStatusWindow(text);
			return
		}

		conf = JSON.parse(text);

		GlobalChangeCounter = data.GlobalChangeCounter;

		document.getElementById('script').value = conf.Script;
		document.getElementById('proxy').value = conf.Proxy;
		document.getElementById('bind').value = conf.Bind;
		document.getElementById('doh').value = conf.Doh;
		$('#networkscope').val(conf.NetworkScope.split(","));
		$('#networkscope').selectpicker("refresh");
		document.getElementById('useragent').value = conf.UserAgent;
		document.getElementById('xforwardedfor').value = conf.XForwardedFor;
		document.getElementById('eventsautorefresh').checked = conf.EventsAutorefresh;
		document.getElementById('eventsautoremove').checked = conf.EventsAutoremove;
		document.getElementById('channelsautoremove').checked = conf.ChannelsAutoremove;
		document.getElementById('maxconcurrentstreams').value = conf.MaxConcurrentStreams.toString();
		document.getElementById('runningmode').value = conf.RunningMode
		document.getElementById('outputmode').value = conf.OutputMode
		document.getElementById('alwaysresetsession').checked = conf.AlwaysResetSession
		document.getElementById('pipeoutputcmdformated').value = conf.PipeOutputCmdFormated
		document.getElementById('norestartonerror').checked = conf.NoRestartOnError
		document.getElementById('restartfinished').checked = conf.RestartFinished
		document.getElementById('norestartontrackchange').checked = conf.NoRestartOnTrackChange
		document.getElementById('reuseeventindex').checked = conf.ReuseEventIndex
		document.getElementById('restartdelay').value = conf.RestartDelay.toString();
		document.getElementById('nbannouncedfragments').value = conf.NbAnnouncedFragments.toString();
		document.getElementById('hlsfragmentsduration').value = conf.HlsFragmentsDuration.toString();
		document.getElementById('playlistduraion').value = conf.PlaylistDuration.toString();
		document.getElementById('autorestartperiod').value = conf.AutorestartPeriod.toString();
		document.getElementById('playbackdelay').value = conf.PlaybackDelay
		document.getElementById('randomautostartperiod').value = conf.RandomAutostartPeriod.toString();
		document.getElementById('sequencialautostartperdiod').value = conf.SequencialAutostartPeriod.toString();
		document.getElementById('epgtimezone').value = conf.EpgTimezone
		document.getElementById('eventsrefreshperiod').value = conf.EventsRefreshPeriod.toString();
		document.getElementById('nowaitfullplaylist').checked = conf.NoWaitFullPlaylist
		document.getElementById('continuousplayback').checked = conf.ContinuousPlayback
		document.getElementById('ignorestaticdash').checked = conf.IgnoreStaticDash
		document.getElementById('usedashdelay').checked = conf.UseDashDelay
		document.getElementById('httpgetretries').value = conf.HttpGetRetries.toString();
		document.getElementById('stalldetecttimeout').value = conf.StallDetectTimeout.toString();
		document.getElementById('vmxuniqueid').value = conf.VmxUniqueId;
		document.getElementById('headers').value = ""
		if (conf.Headers != null) {
			for (i=0; i<conf.Headers.length; i++) {
				document.getElementById('headers').value += conf.Headers[i] + '\n';
			}
		}

		if (conf.Streams) {
			conf.Streams.forEach(function(streamtoupdate){
				table.rows({ page: 'current' }).every( function (rowIdx, tableLoop, rowLoop ) {
					id = /(_[^_]*_[^"]*)/.exec(table.rows().data()[rowIdx][2])[1]

					if (streamtoupdate.Id == getStreamId(id)) {
						if (init) {
							table.cell({row:rowIdx, column:0}).data('<input class="form-check-input" type="checkbox" id="checked' + id + '"/>');
							table.cell({row:rowIdx, column:1}).data('<button style="border-color: transparent;" type="button" class="btn" id="startStop' + id + '"></button>');
							table.cell({row:rowIdx, column:3}).data('<div hidden id="statusfull' + id +'"></div><pre style="white-space: pre-wrap;" id="status' + id +'"></pre><pre style="white-space: pre-wrap;" id="streamerrors' + id +'"></pre><div hidden id="showerrordiv' + id +'"><br><button id="showerror' + id +'" type="button" class="btn btn-outline-primary btn-sm">show more</button></div>');


							celldata = `<aside class="flexname">
								<select class="form-select" style="text-align:center; vertical-align:middle; font-weight: bold" disabled id="mode` + id + `" >
									<option value="live">Live</option>
									<option value="vod">VOD</option>
								</select>
								<div id=modeOverrideDiv` + id + `>
									<hr>
									<select style='text-align:center; vertical-align:middle; font-weight: bold' id="runningmode` + id + `" class="form-select">
										<option value="internalremuxer">Internal remuxer</option>`
							if (!data.InternalUdpOnly) {
								 celldata += `	<option value="residentffmpeg">FFmpeg resident</option>
										<option value="ffmpeg">FFmpeg TS HLS/Direct</option>
										<option value="ffmpegmulti">FFmpeg MultiTS HLS</option>
									     	<option value="fmp4hls">FMP4 HLS</option>`
							}
							celldata +=` 	</select>
									<div id=outputmodeDiv` + id + `>
										<select style='text-align:center; vertical-align:middle; font-weight: bold' id="outputmode` + id + `" class="form-select">`
							if (!data.InternalUdpOnly) {
								celldata += `			<option value="directhls">Direct/HLS</option>
									     			<option value="srtserver">SRT Server</option>`
							}
							celldata +=			       `<option value="protooutput">UDP/SRT</option>
												<option value="custom">Custom</option>
										</select>
									</div>
								</div>
							</aside>`
							table.cell({row:rowIdx, column:4}).data(celldata);
							table.cell({row:rowIdx, column:5}).data(`
							<aside class="flexname">
								<input type="submit" class="btn btn-outline-primary btn-sm" id="showheaders` + id + `" value="show script headers" />
								<br>
								<div id="headersDiv` + id + `">
									<br>
									<textarea class="form-control" disabled" placeholder="manifest headers" id="headersmanifest` + id + `" cols="20" rows="2"></textarea>
									<textarea class="form-control" disabled" placeholder="media headers" id="headersmedia` + id + `" cols="20" rows="2"></textarea>
									<br>
								</div>
								<div class="input-group mb-4">
									<div class="input-group-prepend center-block">
										<input class="form-control" placeholder="param1 param2..." aria-label="manifestScript"  id="manifestscript` + id + `" type="text" value=""/>
										<input class="form-control" placeholder="url" aria-label="manifestURL"  id="manifestUrl` + id + `" type="text" value=""/>
									</div>
									<input  type="submit" class="btn btn-outline-primary btn-sm" name="refresh" id="refresh` + id + `" value="R" />
								</div>
								<div id=networkDiv` + id + `>
									<hr>
									<select class="selectpicker" multiple id="networkscope` + id + `">
										<option value="script">Script</option>
										<option value="manifest">Manifest</option>
										<option value="media">Media files</option>
									</select>
									<br>
									<br>
									<input class="form-control" placeholder="http/socks5 proxy" aria-label="url" id="proxy` + id + `" type="text"/>
									<input class="form-control" placeholder="bind" aria-label="bind" id="bind` + id + `" type="text"/>
									<input class="form-control" placeholder="dns over https" aria-label="url" id="doh` + id + `" type="text"/>
								</div>
								<div style="display: flex;" id="networkInfoDiv` + id + `">
									<div id="networkInfo` + id + `"></div>
								</div>
							</aside>`);
							table.cell({row:rowIdx, column:6}).data(`
							<div style="display: flex; text-align: start;" class="frame" id="manifestInfoDiv` + id + `">
								<img style="object-fit: contain;" id="qualitylogo` + id + `" width="40">
								<div>&nbsp;</div>
								<div>&nbsp;</div>
								<div id="manifestInfo` + id + `"></div>
							</div>
							<br>
							<div class="container mt-2" id="vStreamIdDiv` + id + `">
								<select class="selectpicker" disabled id="vStreamId` + id + `"></select>
							</div>
							<div class="container mt-2" id="aStreamIdDiv` + id + `">
								<select class="selectpicker" disabled multiple id="aStreamId` + id + `"></select>
							</div>
							<div class="container mt-2" id="sStreamIdDiv` + id + `">
								<select class="selectpicker" disabled multiple id="sStreamId` + id + `"></select>
							</div>
							<div id="timeRangeDiv` + id + `">
								<br>
								<div  style="display: flex;">
									<input id="rangeStartDate` + id + `" class="form-control" type="date" />
									<input id="rangeStartTime` + id + `" class="form-control" type="time" />
								</div>
								<div style="display: flex;">
									<input id="rangeEndDate` + id + `" class="form-control" type="date" />
									<input id="rangeEndTime` + id + `" class="form-control" type="time" />
								</div>
							</div>`);
							table.cell({row:rowIdx, column:7}).data(`
							<div id="cdmdiv` + id + `">
								<div style="display: flex;">
									<select disabled class="form-select form-select-sm" style="width:150px; margin:0 auto;display:block;" id="cdmtype` + id + `" >
										<option value="widevine">Widevine</option>
										<option value="playready">Playready</option>
										<option value="verimatrix">Verimatrix</option>
									</select>
									<select disabled class="form-select form-select-sm" style="width:150px; margin:0 auto;display:block;" id="cdmmode` + id + `" >
										<option value="internal">Internal</option>
										<option value="external">External</option>
									</select>
									<input id="prparamsbutton` + id + `" style='width: 20x;' type="submit" value="&#9661;" class="btn btn-primary btn-sm" data-bs-toggle="collapse" data-bs-target="#prparams` + id + `" aria-expanded="false" aria-controls="prparams` + id + `"/>
								</div>
								<br>
								<div id="prparams` + id + `" class="collapse">
									<textarea class="form-control" disabled placeholder="LA version" id="prlaversion` + id + `" cols="39" rows="1"></textarea>
									<textarea class="form-control" disabled placeholder="Client version" id="prclientversion` + id + `" cols="39" rows="1"></textarea>
									<textarea class="form-control" disabled placeholder="Custom data" id="prcustomdata` + id + `" cols="39" rows="1"></textarea>
									<br>
								</div>
								<div style="display: flex;">
									<textarea class="form-control" disabled placeholder="param1 param2..." id="cdm` + id + `" rows="1"></textarea>
									&nbsp;
									<input  type="submit" class="btn btn-outline-primary btn-sm" name="refreshkeys" id="refreshkeys` + id + `" value="R" />
								</div>
							</div>
							<br>
							<div style="display: flex;">
								<textarea class="form-control" disabled placeholder="kid1:key1..." id="keys` + id + `" cols="39" rows="3"></textarea>
								&nbsp;
								<input style="border-color: red; color: red;" type="submit" class="btn btn-outline-primary btn-sm" name="flushkeys" id="flushkeys` + id + `" value="X" />
							</div>
							<br>`);

							content=`<div style="overflow-y: scroll; height: 65%" id="checkboxes` + id + `">
							<div class="form-check" title="Autostart stream on startup.">
								<input class="form-check-input" type="checkbox" id="autostart` + id + `" />
								<label class="form-check-label" for="autostart` + id + `">Autostart</label>
							</div>
							<div class="form-check" title="Start stream only when requested.">
								<input class="form-check-input" disabled type="checkbox" id="ondemand` + id + `" />
								<label class="form-check-label" for="ondemand` + id + `">On demand</label>
							</div>
							<div class="form-check" title="Speed up fragments download. Download them simultaneously.">
								<input class="form-check-input" disabled type="checkbox" id="speedup` + id + `" />
								<label class="form-check-label" for="speedUp` + id + `">Speed up</label>
							</div>
							<div class="form-check" title="Call the provider script to get the manifest. Useful when the manifest comes with a session token.">
								<input class="form-check-input" disabled type="checkbox" id="sessionmanifest` + id + `" />
								<label class="form-check-label" for="sessionManifest` + id + `">Session manifest</label>
							</div>
							<div class="form-check" title="Call the provider script to retrieve stream keys.">
								<input class="form-check-input" disabled type="checkbox" id="usecdm` + id + `" />
								<label class="form-check-label" for="useCdm` + id + `">CDM script</label>
							</div>
							<div class="form-check" title="Override provider network settings">
								<input class="form-check-input" disabled type="checkbox" id="network` + id + `" />
								<label class="form-check-label" for="network` + id + `">Network override</label>
							</div>
							<div class="form-check" title="Override provider running and output modes">
								<input class="form-check-input" disabled type="checkbox" id="modeoverride` + id + `" />
								<label class="form-check-label" for="network` + id + `">Mode override</label>
							</div>
							<div class="form-check" title="Record event and create a replay entry.">
								<input class="form-check-input" type="checkbox" id="recordevent` + id + `" />
								<label class="form-check-label" for="recordevent` + id + `">Record event</label>
							</div>
							<div class="form-check" title="Ignore channels/events updates for this stream.">
								<input class="form-check-input" type="checkbox" id="ignoreupdate` + id + `" />
								<label class="form-check-label" for="ignoreUpdate` + id + `">Ignore update</label>
							</div>
							<div class="form-check" title="Fix IV size for some providers.">
								<input class="form-check-input" disabled type="checkbox" id="fixivsize` + id + `" />
								<label class="form-check-label" for="fixivsize` + id + `">Fix IV size</label>
							</div>
							<div class="form-check" title="Use time range. VOD mode must be set.">
								<input class="form-check-input" disabled type="checkbox" id="timerange` + id + `" />
								<label class="form-check-label" for="timerange` + id + `">Time range</label>
							</div>
							</div>
							<div id="pipeoutputparamsDiv` + id + `">
								<div class="input-group mb-3">
									<span style='width: 65px;' class="input-group-text">Pipe</span>
									<input class="form-control" placeholder="params" aria-label="params" id="pipeoutputparams` + id + `" type="text"/>
								</div>
							</div>
							<div id="protooutputparamsDiv` + id + `">
								<div class="input-group mb-3">
									<span style='width: 65px;' class="input-group-text">UDP/SRT</span>
									<input class="form-control" placeholder="params" aria-label="params" id="protooutputparams` + id + `" type="text"/>
								</div>
							</div>`
							table.cell({row:rowIdx, column:8}).data(content);

							document.getElementById('name' + id).onclick = setPlayer;
							document.getElementById('renamestream' + id).onclick = renameStreamClick;
							document.getElementById('showerror' + id).onclick = showErrorLogWindow;
							document.getElementById('mode' + id).onchange = updateStream;
							document.getElementById('runningmode' + id).onchange = updateStream;
							document.getElementById('outputmode' + id).onchange = updateStream;
							document.getElementById('manifestscript' + id).onchange = updateStream;
							document.getElementById('manifestUrl' + id).onchange = updateStream;
							document.getElementById('showheaders' + id).onclick = showHideHeaders;
							$('#vStreamId' + id).on('hidden.bs.select', updateStream);
							$('#aStreamId' + id).on('hidden.bs.select', updateStream);
							$('#sStreamId' + id).on('hidden.bs.select', updateStream);
							document.getElementById('cdm' + id).onchange = updateStream;
							document.getElementById('cdmtype' + id).onchange = updateStream;
							document.getElementById('cdmmode' + id).onchange = updateStream;
							document.getElementById('prlaversion' + id).onchange = updateStream;
							document.getElementById('prclientversion' + id).onchange = updateStream;
							document.getElementById('prcustomdata' + id).onchange = updateStream;
							document.getElementById('keys' + id).onchange = updateStream;
							document.getElementById('autostart' + id).onchange = updateStream;
							document.getElementById('ondemand' + id).onchange = updateStream;
							document.getElementById('pipeoutputparams' + id).onchange = updateStream;
							document.getElementById('protooutputparams' + id).onchange = updateStream;
							document.getElementById('sessionmanifest' + id).onchange = updateStream;
							document.getElementById('speedup' + id).onchange = updateStream;
							document.getElementById('usecdm' + id).onchange = updateStream;
							document.getElementById('network' + id).onchange = updateStream;
							document.getElementById('modeoverride' + id).onchange = updateStream;
							document.getElementById('recordevent' + id).onchange = updateStream;
							document.getElementById('ignoreupdate' + id).onchange = updateStream;
							document.getElementById('fixivsize' + id).onchange = updateStream;
							document.getElementById('timerange' + id).onchange = updateStream;
							document.getElementById('bind' + id).onchange = updateStream;
							document.getElementById('proxy' + id).onchange = updateStream;
							document.getElementById('doh' + id).onchange = updateStream;
							document.getElementById('networkscope' + id).onchange = updateStream;
							document.getElementById('rangeStartDate' + id).onchange = updateStream;
							document.getElementById('rangeStartTime' + id).onchange = updateStream;
							document.getElementById('rangeEndDate' + id).onchange = updateStream;
							document.getElementById('rangeEndTime' + id).onchange = updateStream;
							document.getElementById('startStop' + id).onclick = startStopClick;
							document.getElementById('refresh' + id).onclick = refreshClick;
							document.getElementById('refreshkeys' + id).onclick = refreshKeysClick;
							document.getElementById('flushkeys' + id).onclick = flushKeysClick;
						}

						populateStream(streamtoupdate)
					}
				});
			});
		}
	}

	// Update status
	if (data.Providers) {
		if (document.getElementById('runningmode').value == 'ffmpeg' || document.getElementById('runningmode').value == 'ffmpegmulti' || document.getElementById('runningmode').value == 'fmp4hls') {
			document.getElementById('outputmodediv').style.display = 'none';
		} else {
			document.getElementById('outputmodediv').style.display = 'flex';
		}
		data.Providers.forEach(function(provider){
			if (provider.Streams) {
				provider.Streams.forEach(function(stream){
					let n = '_' + provider.Id + '_' + stream.Id;

					if (document.getElementById('name' + n)) {
						if (!document.getElementById('startStop' + n)) {
							return
						}

						document.getElementById('name' + n).href = stream.StreamingUrl;

						if (stream.Running) {
							if (!document.getElementById('startStop' + n).innerHTML.includes('stop.svg')) {
								document.getElementById('startStop' + n).innerHTML = '<img style="width: 40px; height: auto;" src="static/stop.svg"/>';
							}
							document.getElementById('name' + n).style.borderColor = "green";
						} else {
							if (!document.getElementById('startStop' + n).innerHTML.includes('play.svg')) {
								document.getElementById('startStop' + n).innerHTML = '<img style="width: 40px; height: auto;" src="static/play.svg"/>';
							}
							document.getElementById('name' + n).style.borderColor = "grey";
						}

						if (stream.Status.includes('Error')) {
							document.getElementById('name' + n).style.borderColor = "red";
						} else if (stream.Status.includes('Key(s) missing')) {
							document.getElementById('name' + n).style.borderColor = "yellow";
						}
		
						document.getElementById('statusfull' + n).textContent = stream.Status;
						if (stream.Status.includes("Error:") || stream.Status.includes('Key(s) missing')) {
							document.getElementById('status' + n).textContent = stream.Status.substring(0, 20).substring(0, stream.Status.indexOf('\n'));
							if (stream.Status.length > 20  || stream.Status.includes('\n')) {
								document.getElementById('showerrordiv' + n).removeAttribute("hidden");
								if (!stream.Status.includes('autorestart in')) {
									document.getElementById('status' + n).textContent += "..."
								}
							} else {
								document.getElementById('showerrordiv' + n).setAttribute("hidden","hidden");
							}
						} else {
							document.getElementById('status' + n).textContent = stream.Status;
							document.getElementById('showerrordiv' + n).setAttribute("hidden","hidden");
						}
						document.getElementById('status' + n).style.color = stream.StatusColor;
						document.getElementById('status' + n).title = stream.StreamInfo;
						if (stream.StreamErrors != '') {
							document.getElementById('streamerrors' + n).style.display = 'block';
							document.getElementById('streamerrors' + n).textContent = stream.StreamErrors;
							document.getElementById('streamerrors' + n).style.color = 'yellow';
							document.getElementById('status' + n).textContent = "\n\n\n" + document.getElementById('status' + n).textContent;
						} else {
							document.getElementById('streamerrors' + n).style.display = 'none';
						}

						if (stream.CurrentVideo == '') {
							setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/na.jpg");
						} else	if (stream.CurrentVideo.includes("2160p")) {
							setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/4k.jpg");
						} else if (stream.CurrentVideo.includes("1440p")) {
							setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/2k.jpg");
						} else if (stream.CurrentVideo.includes("1080p")) {
							setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/1080p.jpg");
						} else if (stream.CurrentVideo.includes("720p")) {
							setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/720p.jpg");
						} else  {
							setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/sd.jpg");
						}

						document.getElementById('autostart' + n).removeAttribute("disabled");

						if (stream.Running) {
							document.getElementById('ondemand' + n).setAttribute("disabled","disabled");
							document.getElementById('mode' + n).setAttribute("disabled","disabled");
							document.getElementById('manifestUrl' + n).setAttribute("disabled","disabled");
							document.getElementById('manifestscript' + n).setAttribute("disabled","disabled");
							document.getElementById('proxy' + n).setAttribute("disabled","disabled");
							document.getElementById('bind' + n).setAttribute("disabled","disabled");
							document.getElementById('doh' + n).setAttribute("disabled","disabled");
							document.getElementById('networkscope' + n).setAttribute("disabled","disabled");
							$('#networkscope' + n).selectpicker("refresh");
							document.getElementById('runningmode' + n).setAttribute("disabled","disabled");
							document.getElementById('outputmode' + n).setAttribute("disabled","disabled");
							document.getElementById('refresh' + n).setAttribute("disabled","disabled");
							document.getElementById('vStreamId' + n).setAttribute("disabled","disabled");
							document.getElementById('aStreamId' + n).setAttribute("disabled","disabled");
							document.getElementById('sStreamId' + n).setAttribute("disabled","disabled");
							document.getElementById('keys' + n).setAttribute("disabled","disabled");
							document.getElementById('cdmtype' + n).setAttribute("disabled","disabled");
							document.getElementById('cdmmode' + n).setAttribute("disabled","disabled");
							document.getElementById('cdm' + n).setAttribute("disabled","disabled");
							document.getElementById('prlaversion' + n).setAttribute("disabled","disabled");
							document.getElementById('prclientversion' + n).setAttribute("disabled","disabled");
							document.getElementById('prcustomdata' + n).setAttribute("disabled","disabled");
							document.getElementById('refreshkeys' + n).setAttribute("disabled","disabled");
							document.getElementById('flushkeys' + n).setAttribute("disabled","disabled");
							document.getElementById('sessionmanifest' + n).setAttribute("disabled","disabled");
							document.getElementById('speedup' + n).setAttribute("disabled","disabled");
							document.getElementById('usecdm' + n).setAttribute("disabled","disabled");
							document.getElementById('network' + n).setAttribute("disabled","disabled");
							document.getElementById('recordevent' + n).setAttribute("disabled","disabled");
							document.getElementById('modeoverride' + n).setAttribute("disabled","disabled");
							document.getElementById('fixivsize' + n).setAttribute("disabled","disabled");
							document.getElementById('timerange' + n).setAttribute("disabled","disabled");
							document.getElementById('pipeoutputparams' + n).setAttribute("disabled","disabled");
							document.getElementById('protooutputparams' + n).setAttribute("disabled","disabled");
						} else {
							document.getElementById('ondemand' + n).removeAttribute("disabled");
							document.getElementById('mode' + n).removeAttribute("disabled");
							document.getElementById('manifestUrl' + n).removeAttribute("disabled");
							document.getElementById('manifestscript' + n).removeAttribute("disabled");
							document.getElementById('proxy' + n).removeAttribute("disabled");
							document.getElementById('bind' + n).removeAttribute("disabled");
							document.getElementById('doh' + n).removeAttribute("disabled");
							document.getElementById('networkscope' + n).removeAttribute("disabled");
							$('#networkscope' + n).selectpicker("refresh");
							document.getElementById('runningmode' + n).removeAttribute("disabled");
							document.getElementById('outputmode' + n).removeAttribute("disabled");
							document.getElementById('refresh' + n).removeAttribute("disabled");
							document.getElementById('vStreamId' + n).removeAttribute("disabled");
							document.getElementById('aStreamId' + n).removeAttribute("disabled");
							document.getElementById('sStreamId' + n).removeAttribute("disabled");
							document.getElementById('keys' + n).removeAttribute("disabled");
							document.getElementById('cdmtype' + n).removeAttribute("disabled");
							document.getElementById('cdmmode' + n).removeAttribute("disabled");
							document.getElementById('cdm' + n).removeAttribute("disabled");
							document.getElementById('prlaversion' + n).removeAttribute("disabled");
							document.getElementById('prclientversion' + n).removeAttribute("disabled");
							document.getElementById('prcustomdata' + n).removeAttribute("disabled");
							document.getElementById('refreshkeys' + n).removeAttribute("disabled");
							document.getElementById('flushkeys' + n).removeAttribute("disabled");
							document.getElementById('sessionmanifest' + n).removeAttribute("disabled");
							document.getElementById('speedup' + n).removeAttribute("disabled");
							document.getElementById('usecdm' + n).removeAttribute("disabled");
							document.getElementById('network' + n).removeAttribute("disabled");
							document.getElementById('recordevent' + n).removeAttribute("disabled");
							document.getElementById('modeoverride' + n).removeAttribute("disabled");
							document.getElementById('fixivsize' + n).removeAttribute("disabled");
							document.getElementById('timerange' + n).removeAttribute("disabled");
							document.getElementById('pipeoutputparams' + n).removeAttribute("disabled");
							document.getElementById('protooutputparams' + n).removeAttribute("disabled");
						}

						if (document.getElementById('modeoverride' + n).checked) {
							if (document.getElementById('runningmode' + n).value == 'ffmpeg' || document.getElementById('runningmode' + n).value == 'ffmpegmulti' || document.getElementById('runningmode' + n).value == 'fmp4hls') {
								document.getElementById('outputmodeDiv' + n).style.display = 'none';
							} else {
								document.getElementById('outputmodeDiv' + n).style.display = 'flex';
							}

							runningmode = stream.RunningMode
							outputmode = stream.OutputMode
						} else {
							runningmode = provider.RunningMode
							outputmode = provider.OutputMode
						}

						if ( runningmode == "ffmpeg" || runningmode == "fmp4hls" || runningmode == "ffmpegmulti") {
							document.getElementById('checkboxes' + n).style.height = '100%';
							document.getElementById('pipeoutputparamsDiv' + n).style.display = 'none';
							document.getElementById('protooutputparamsDiv' + n).style.display = 'none';
						} else {
							if ( outputmode == "srtserver" || outputmode == "directhls") {
								document.getElementById('checkboxes' + n).style.height = '100%';
								document.getElementById('pipeoutputparamsDiv' + n).style.display = 'none';
								document.getElementById('protooutputparamsDiv' + n).style.display = 'none';
							} else if (outputmode == "custom") {
								document.getElementById('checkboxes' + n).style.height = '80%';
								document.getElementById('pipeoutputparamsDiv' + n).style.display = 'flex';
								document.getElementById('protooutputparamsDiv' + n).style.display = 'none';
							} else if (outputmode == "protooutput") {
								document.getElementById('checkboxes' + n).style.height = '80%';
								document.getElementById('pipeoutputparamsDiv' + n).style.display = 'none';
								document.getElementById('protooutputparamsDiv' + n).style.display = 'flex';
							}
						}


						if (stream.Mode == "vod") {
							document.getElementById('autostart' + n).setAttribute("disabled","disabled");
							document.getElementById('ondemand' + n).setAttribute("disabled","disabled");
							document.getElementById('speedup' + n).setAttribute("disabled","disabled");
						}

						if (outputmode != "directhls" && outputmode != "srtserver" && runningmode != "fmp4hls" && runningmode != "ffmpegmulti") {
							document.getElementById('ondemand' + n).setAttribute("disabled","disabled");
						}

						if (document.getElementById('timerange' + n) && stream.Mode != "vod") {
							document.getElementById('timerange' + n).setAttribute("disabled","disabled");
						}

						if (stream.Mode == "vod" && document.getElementById('timerange' + n) && document.getElementById('timerange' + n).checked) {
							document.getElementById('timeRangeDiv' + n).style.display = 'block';
						} else {
							document.getElementById('timeRangeDiv' + n).style.display = 'none';
						}

						$('#vStreamId' + n).selectpicker("refresh");
						$('#aStreamId' + n).selectpicker("refresh");
						$('#sStreamId' + n).selectpicker("refresh");

						text = 'Please refresh for more info';

						if (stream.ManifestInfo != '') {
							text = stream.ManifestInfo;
						}
						if (text.includes('<br>')) {
							document.getElementById('manifestInfo'+ n).innerHTML = text;
						} else {
							document.getElementById('manifestInfo'+ n).innerText = text;
						}

						if (document.getElementById('cdmtype' + n).options[document.getElementById('cdmtype' + n).selectedIndex].value == "playready" &&
							document.getElementById('cdmmode' + n).options[document.getElementById('cdmmode' + n).selectedIndex].value != "external" ) {
							document.getElementById('prparamsbutton' + n).style.display = 'flex';
						} else {
							document.getElementById('prparamsbutton' + n).style.display = 'none';
						}

						document.getElementById('networkInfoDiv' + n).style.display = 'none';
						if (document.getElementById('network' + n).checked) {
							document.getElementById('networkDiv' + n).style.display = 'block';
						} else {
							document.getElementById('networkDiv' + n).style.display = 'none';
							if (stream.Running && (stream.CurrentBind || stream.CurrentProxy || stream.CurrentDoh)) {
								document.getElementById('networkInfoDiv' + n).style.display = 'block';
								var text = "";
								if (stream.CurrentBind) {
									text += "Bind: " + stream.CurrentBind + "\n";
								}
								if (stream.CurrentProxy) {
									text += "Proxy: " + stream.CurrentProxy + "\n";
								}
								if (stream.CurrentDoh) {
									text += "Doh: " + stream.CurrentDoh + "\n";
								}
								text += "Used for: " + stream.CurrentNetworkScope + "\n";
								document.getElementById('networkInfo'+ n).innerText = text;
							}
						}

						document.getElementById('modeOverrideDiv' + n).style.display = 'none';
						if (document.getElementById('modeoverride' + n).checked) {
							document.getElementById('modeOverrideDiv' + n).style.display = 'block';
						} else {
							document.getElementById('modeOverrideDiv' + n).style.display = 'none';
						}

						if (!stream.IsEvent) {
							document.getElementById('recordevent' + n).setAttribute("disabled","disabled");
						}
					}
				});
			}
		});
	}
}

async function updateStatus() {

	if (document.hidden) {
		return
	}

	if (document.getElementById('providerSelection').selectedIndex >= 0) {
		providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value;
	} else {
		providerId = 'all'
	}
	let response = await fetch(document.getElementById('baseUrl').value + 'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
				'Content-Type': 'application/json',
		},
		body: JSON.stringify({
					Action : "getstatus",
					ProviderId : providerId,
					Streams: [  { Id : "all", }, ]
		}),
	});

	let data = "";

	if (!response.ok) {	
		return
	} else {
		resp = await response.text();
		if (resp == "") {
			return;
		}
		data = JSON.parse(resp);
		if (!data.Providers) {
			return
		}
	}

	data.Providers.forEach(function(provider){
		if (provider.Streams) {
			provider.Streams.forEach(function(stream){
				let n = '_' + provider.Id + '_' + stream.Id;
				if (document.getElementById('name' + n)) {
					document.getElementById('name' + n).href = stream.StreamingUrl;

					if (stream.Running || stream.OnDemand || stream.Mode != "live") {
						document.getElementById('name' + n).onclick = setPlayer;
					} else {
						document.getElementById('name' + n).onclick = noPlayer;
					}

					if (stream.Status.includes("Error:")) {
						document.getElementById('name' + n).style.borderColor = "red";
					} else if (stream.Status.includes('Key(s) missing')) {
						document.getElementById('name' + n).style.borderColor = "yellow";
					} else {
						if (stream.Running) {
							document.getElementById('name' + n).style.borderColor = "green";
						} else {
							document.getElementById('name' + n).style.borderColor = "grey";
						}
					}

					document.getElementById('name' + n).style.color = stream.NameColor;
					document.getElementById('statusfull' + n).textContent = stream.Status;
					if (stream.Status.includes("Error:") || stream.Status.includes('Key(s) missing')) {
						document.getElementById('status' + n).textContent = stream.Status.substring(0, 20).substring(0, stream.Status.indexOf('\n'));
						if (stream.Status.length > 20  || stream.Status.includes('\n')) {
							if (!stream.Status.includes('autorestart in')) {
								document.getElementById('status' + n).textContent += "..."
							}
						}
					} else {
						document.getElementById('status' + n).textContent = stream.Status;
					}
					document.getElementById('status' + n).style.color = stream.StatusColor;
					document.getElementById('status' + n).title = stream.StreamInfo;
					document.getElementById('status' + n).onclick = showErrorLogWindow;
					document.getElementById('streamerrors' + n).textContent = stream.StreamErrors;
					document.getElementById('streamerrors' + n).style.color = 'yellow';
					if (stream.CurrentVideo == '') {
						document.getElementById('qualitylogo' + n).style.display = 'none';
					} else if (stream.CurrentVideo.includes("2160p")) {
						setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/4k.jpg");
						document.getElementById('qualitylogo' + n).style.display = 'inline-block';
					} else if (stream.CurrentVideo.includes("1440p")) {
						setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/2k.jpg");
						document.getElementById('qualitylogo' + n).style.display = 'inline-block';
					} else if (stream.CurrentVideo.includes("1080p")) {
						setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/1080p.jpg");
						document.getElementById('qualitylogo' + n).style.display = 'inline-block';
					} else if (stream.CurrentVideo.includes("720p")) {
						setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/720p.jpg");
						document.getElementById('qualitylogo' + n).style.display = 'inline-block';
					} else  {
						setImgSrc(document.getElementById('qualitylogo' + n), document.getElementById('baseUrl').value + "static/sd.jpg");
						document.getElementById('qualitylogo' + n).style.display = 'inline-block';
					}
					if (document.getElementById('startStop' + n)) {
						if (stream.Running) {
							if (!document.getElementById('startStop' + n).innerHTML.includes('stop.svg')) {
								document.getElementById('startStop' + n).innerHTML = '<img style="width: 20px; height: auto;" src="static/stop.svg"/>';
								document.getElementById('name' + n).style.borderColor = "green";
							}
						} else {
							if (!document.getElementById('startStop' + n).innerHTML.includes('play.svg')) {
								document.getElementById('startStop' + n).innerHTML = '<img style="width: 20px; height: auto;" src="static/play.svg"/>';
								document.getElementById('name' + n).style.borderColor = "grey";
							}
						}
						document.getElementById('startStop' + n).onclick = startStopClick;
					}
				}
			});
		}
	});
}

async function updateMonitoring(table) {

	if (document.hidden) {
		return
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify({
					Action : "getmonitoring",
					}),
	}).catch(error => { showStatusWindow('remote server unreachable'); clearInterval(intervalId); intervalId = 0; });

	if (intervalId == 0) {
		return
	}

	let data = "";

	text = await response.text();
	if (!response.ok) {
		showStatusWindow(text);
		return
	}

	data = JSON.parse(text);

	document.getElementById('totalBwInText').value = data.TotalBwIn;
	document.getElementById('totalBwOutText').value = data.TotalBwOut;
	document.getElementById('cpuLoadText').value = data.CpuLoad;
	document.getElementById('cpuLoadText').style.color = data.CpuLoadColor;
	document.getElementById('memoryText').value = data.Memory;

	table.rows().remove()

	// CHECK ADD/DELETE
	if (!data.Readers) {
		table.draw();
		return
	}

	data.Readers.forEach(function(reader){
		row = table.row.add([	reader.StreamName,
					reader.ProviderName,
					reader.Type,
					reader.User,
					reader.Ip,
					reader.UserAgent,
					reader.Uptime,
					'<a style="color:' + reader.ErrorsColor + ';">' + reader.Errors  + '</a>',
					'<a style="color:' + reader.BwColor + ';">' + reader.Bw  + '</a>'
					]).node()
	});

	table.draw();

}

async function updateRecordings() {

	if (document.hidden) {
		return
	}

	let response = await fetch(document.getElementById('baseUrl').value + 'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
				'Content-Type': 'application/json',
		},
		body: JSON.stringify({ Action : "getrecordings", Streams: [  { Id : "all", }, ] }),
	});

	let data = "";

	if (!response.ok) {	
		return
	} else {
		resp = await response.text();
		if (resp == "") {
			return;
		}
		data = JSON.parse(resp);
		if (!data.Recording && !table.rows().count()) {
			return
		}
	}

	// CHECK ADD/DELETE
	if ((!data.Recording && table.rows().count() > 0) || (data.Recording.length != table.rows().count())) {
		clearInterval(intervalId);
		location.reload();
	}

	data.Recording.forEach(function(item){
		// Find index
		let n = 0
		let found = false
		while (true) {
			if (document.getElementById('recid' + n) == null) {
				break
			}
			if (document.getElementById('recid' + n).value == item.Id) {
				found = true
				break
			}

			n++
		}

		if (found) {
			document.getElementById('recstatus' + n).textContent = item.Status;
			document.getElementById('recstatus' + n).style.color = item.StatusColor;
		}
	});
}

async function updateRecordingChannels() {

	if (document.hidden) {
		return
	}

	if (document.getElementById('providerSelection').selectedIndex >= 0) {
		providerId = document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value;
	} else {
		providerId = 'all'
	}
	let response = await fetch(document.getElementById('baseUrl').value + 'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
				'Content-Type': 'application/json',
		},
		body: JSON.stringify({
					Action : "getstatus",
					ProviderId : providerId,
					Streams: [  { Id : "all", }, ]
		}),
	});

	let data = "";

	if (!response.ok) {	
		return
	} else {
		resp = await response.text();
		if (resp == "") {
			return;
		}
		data = JSON.parse(resp);
		if (!data.Providers) {
			return
		}
	}

	var streamsSelector = 'addRecChan'
	$('#' + streamsSelector).empty();
	data.Providers.forEach(function(provider){
		if (provider.Streams) {
			provider.Streams.forEach(function(stream){
				$('#' + streamsSelector).append('<option value="' + stream.Id + '">'+stream.Name + '</option>');
			})
		}
	})
}


function makeid(length) {
    var result           = '';
    var characters       = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    var charactersLength = characters.length;
    for ( var i = 0; i < length; i++ ) {
      result += characters.charAt(Math.floor(Math.random() * 
 charactersLength));
   }
   return result;
}

// ADD REC onCLick
async function addRecFormSubmit(e) {
	e.preventDefault();

	startTime = document.getElementById('recStartTime').value
	endTime = document.getElementById('recEndTime').value
	if (startTime == '') {
		startTime = "00:00"
	}
	if (endTime == '') {
		endTime = "00:00"
	}

        try {
                start = new Date(document.getElementById('recStartDate').value + 'T' + startTime + ":00").toISOString()
        } catch (error) {
                start = ""
        }
        try {
                end = new Date(document.getElementById('recEndDate').value + 'T' + endTime + ":00").toISOString()
        } catch (error) {
                end = ""
        }

	data = {
		Action : "addrec",
		Recordings: [
			{
				Id: makeid(15),
				StreamName : document.getElementById('addRecChan').options[document.getElementById('addRecChan').selectedIndex].innerText,
				StreamId : document.getElementById('addRecChan').options[document.getElementById('addRecChan').selectedIndex].value,
				ProviderId : document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value,
				Start: start,
				End: end,
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		document.getElementById('recActionStatus').innerText = "[" + response.statusText + "] " + text ;
		document.getElementById('recActionStatus').style.color = 'red';
	} else {
		location.reload();
	}
};

// DELETE onCLick
async function recDeleteSubmit(e) {
	e.preventDefault();

	var rowArray = table.rows({ page: 'current' }).data().toArray();
	if (rowArray == null) {
		return
	}

	var idsList = ""
 	var namesList = ""
	rowArray.forEach( function (row) {
		if (document.getElementById('recchecked' + /id="recstreamname(.*?)"/.exec(row[2])[1]).checked) {
			idsList += /value="(.*?)"/.exec(row[2])[1] + ","
			namesList +=  document.getElementById('recstreamname' + /id="recstreamname(.*?)"/.exec(row[2])[1]).innerText + "\n"
		}
	});

	namesList = namesList.slice(0, -1)

	idsList = idsList.slice(0, -1)
	if (idsList == "") {
		return;
	}

	var result = confirm("Are you sure you want to delete:\n\n" +  namesList);
        if (result != true) {
                return
        }

	data = {
		Action : "deleterec",
		Recordings: [
			{
				Id : idsList
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		document.getElementById('recActionStatus').innerText = "[" + response.statusText + "] " + text ;
		document.getElementById('recActionStatus').style.color = 'red';
	} else {
		location.reload();
	}
};

// DELETE onCLick
async function recStopSubmit(e) {
	e.preventDefault();

	var rowArray = table.rows({ page: 'current' }).data().toArray();
	if (rowArray == null) {
		return
	}

	var idsList = ""
	var namesList = ""
	rowArray.forEach( function (row) {
		if (document.getElementById('recchecked' + /id="recstreamname(.*?)"/.exec(row[2])[1]).checked) {
			idsList += /value="(.*?)"/.exec(row[2])[1] + ","
			namesList +=  document.getElementById('recstreamname' + /id="recstreamname(.*?)"/.exec(row[2])[1]).innerText + "\n"
		}
	});

	namesList = namesList.slice(0, -1)

	idsList = idsList.slice(0, -1)
	if (idsList == "") {
		return;
	}

	var result = confirm("Are you sure you want to stop:\n\n" +  namesList);
        if (result != true) {
                return
        }

	data = {
		Action : "stoprec",
		Recordings: [
			{
				Id : idsList
			},
		]
	}

	let response = await fetch(document.getElementById('baseUrl').value+'api/action', {
		method: 'POST', // or 'PUT'
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify(data),
	})

	if (!response.ok) {
		text = await response.text();
		document.getElementById('recActionStatus').innerText = "[" + response.statusText + "] " + text ;
		document.getElementById('recActionStatus').style.color = 'red';
	} else {
		location.reload();
	}
};

function showHideHeaders(e) {
	e.preventDefault();
	n = getId(this.id)

	var x = document.getElementById("headersDiv" + n);
	if (x.style.display === "none") {
		x.style.display = "block";
		document.getElementById("showheaders" + n).value='hide script headers';
	} else {
		x.style.display = "none";
		document.getElementById("showheaders" + n).value='show script headers';
	}
}

// showing loading
function displayLoading(loader) {
	loader.classList.add("display");

	// to stop loading after some time
	//setTimeout(() => {
        //	loader.classList.remove("display");
	//}, 5000);
}

// hiding loading 
function hideLoading(loader) {
	loader.classList.remove("display");
}

function filterOnChange() {
	if (document.URL.includes("/config")) {
		page = "config"
	} else {
		page = "status"
	}

	filter = document.getElementById('filterselection').options[document.getElementById('filterselection').selectedIndex].value;
	if (document.getElementById('providerSelection').selectedIndex >= 0 ) {
		window.location.href = "/" + page + "/" + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value + "?filter=" + filter;
	} else {
		window.location.href = "/" + page + "/" + document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value; 
	}
}

async function filterDeleteOnClick(e) {
	e.preventDefault();

	if (document.URL.includes("/config")) {
		page = "config"
	} else {
		page = "status"
	}

	if (document.getElementById('providerSelection').selectedIndex >= 0 ) {
		window.location.href = "/" + page + "/" +  document.getElementById('providerSelection').options[document.getElementById('providerSelection').selectedIndex].value;
	} else {
		window.location.href = "/" + page + "/";
	}

}

function showErrorLogWindow(e, id) {
	e.preventDefault();

        if (!id) {
                id = this.id
	}
	
	id = getId(id)

	if (!document.getElementById('statusfull' + id).textContent.includes("Error:") && !document.getElementById('statusfull' + id).textContent.includes("Key(s) missing")) {
		return
	}

	bootbox.alert( { title:"Error description" , message: '<pre>' + document.getElementById('statusfull' + id).textContent + '</pre>'});
}

function showStatusWindow(error) {
	bootbox.alert( { title:"Error" , message: '<pre>' + error + '</pre>'});
}

function getSelectedIds() {
	var rowArray = table.rows({ page: 'current' }).data().toArray();
	if (rowArray == null) {
		return ""
	}

	var idsList = ""
	rowArray.forEach( function (row) {
		id = /name(_[^_]*_[^"]*)/.exec(row[2])[1]

		if ((document.getElementById('checked' + id).checked)) {
			idsList += getStreamId(id) + ","
		}
	});

	idsList = idsList.slice(0, -1)

	return idsList;
}

function getVisibleIds(init) {
	var rowArray = table.rows({ page: 'current' }).data().toArray();
	if (rowArray == null) {
		return ""
	}

	var idsList = ""
	rowArray.forEach( function (row) {
		var m = /name(_[^_]*_[^"]*)/.exec(row[2])
		if (!m) {
			return
		}
		id = m[1]
		var el = document.getElementById('name' + id)
		if ((el && isInViewport(el)) || (init && !document.getElementById('startStop' + id)) ) {
			idsList += getStreamId(id) + ","
		}
	});

	idsList = idsList.slice(0, -1)

	return idsList;
}

function getPageIds(selectedOnly) {
	var rowArray = table.rows({ page: 'current' }).data().toArray();
	if (rowArray == null) {
		return ""
	}

	var idsList = ""
	rowArray.forEach( function (row) {
		id = /name(_[^_]*_[^"]*)/.exec(row[2])[1]
		idsList += getStreamId(id) + ","
	});

	idsList = idsList.slice(0, -1)

	return idsList;
}

function setImgSrc(img, src) {
	if (!img.src.includes(src)) {
		img.src = src;

	}
}


