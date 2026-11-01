package io.github.phantomconartist.mace

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_WEAK
import androidx.biometric.BiometricManager.Authenticators.DEVICE_CREDENTIAL
import androidx.biometric.BiometricPrompt
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.phantomconartist.mace.ui.ChatActions
import io.github.phantomconartist.mace.ui.ChatScreen
import io.github.phantomconartist.mace.ui.HomeActions
import io.github.phantomconartist.mace.ui.HomeScreen
import io.github.phantomconartist.mace.ui.InfoActions
import io.github.phantomconartist.mace.ui.InfoPage
import io.github.phantomconartist.mace.ui.LinkScreen
import io.github.phantomconartist.mace.ui.LinkedDevicesPage
import io.github.phantomconartist.mace.ui.LocalVerify
import io.github.phantomconartist.mace.ui.NewChatPage
import io.github.phantomconartist.mace.ui.NewGroupPage
import io.github.phantomconartist.mace.ui.PolicySheet
import io.github.phantomconartist.mace.ui.StartAgentPage
import io.github.phantomconartist.mace.ui.theme.MaceTheme

class MainActivity : FragmentActivity() {
    private val model: AppModel by viewModels()
    private val shared = mutableStateOf<Uri?>(null)

    /** Asks for the fingerprint, face, or screen lock; a phone without one goes straight on. */
    private fun verify(title: String, ok: () -> Unit) {
        val kinds = BIOMETRIC_WEAK or DEVICE_CREDENTIAL
        if (BiometricManager.from(this).canAuthenticate(kinds) != BiometricManager.BIOMETRIC_SUCCESS) return ok()
        BiometricPrompt(this, ContextCompat.getMainExecutor(this), object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) = ok()
        }).authenticate(BiometricPrompt.PromptInfo.Builder().setTitle(title).setSubtitle("Confirm it is you").setAllowedAuthenticators(kinds).build())
    }

    private fun takeShare(intent: Intent?) {
        intent?.getStringExtra(Notify.EXTRA_STATUS)?.let { model.showStatus(it) }
        if (intent?.action == Intent.ACTION_SEND) {
            @Suppress("DEPRECATION")
            shared.value = intent.getParcelableExtra(Intent.EXTRA_STREAM)
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        takeShare(intent)
    }

    // Notifications are only for when MACE is not on screen.
    override fun onStart() { super.onStart(); model.foreground = true }
    override fun onStop() { super.onStop(); model.foreground = false }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        takeShare(intent)
        setContent {
            MaceTheme {
                CompositionLocalProvider(LocalVerify provides ::verify) {
                    Surface { App() }
                }
            }
        }
    }

    @androidx.compose.runtime.Composable
    private fun App() {
        val screen by model.screen.collectAsStateWithLifecycle()
        val chat by model.chat.collectAsStateWithLifecycle()
        val agents by model.agents.collectAsStateWithLifecycle()
        val rooms by model.rooms.collectAsStateWithLifecycle()
        val notice by model.notice.collectAsStateWithLifecycle()
        val observe by model.observe.collectAsStateWithLifecycle()
        val linked by model.linked.collectAsStateWithLifecycle()
        var page by rememberSaveable { mutableStateOf("home") }
        var info by rememberSaveable { mutableStateOf<String?>(null) }
        var mode by remember { mutableStateOf(false) }
        val bar = androidx.compose.material3.MaterialTheme.colorScheme.surface
        val webFiles: () -> Unit = { model.webUrl("files")?.let { url -> io.github.phantomconartist.mace.ui.openWeb(this@MainActivity, url, android.graphics.Color.argb(255, (bar.red * 255).toInt(), (bar.green * 255).toInt(), (bar.blue * 255).toInt())) } }
        var photoFor by rememberSaveable { mutableStateOf<Pair<String, Boolean>?>(null) }
        val share by shared
        val snack = remember { SnackbarHostState() }

        val gallery = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri -> uri?.let { model.upload(it) } }
        val document = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri -> uri?.let { model.upload(it) } }
        val photo = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri ->
            val target = photoFor
            if (uri != null && target != null) model.setPhoto(target.first, target.second, uri)
            photoFor = null
        }
        val open: (String) -> Unit = { path -> model.openFile(path) { startActivity(it) } }

        LaunchedEffect(notice) { notice?.let { snack.showSnackbar(it); model.noticeShown() } }
        // Ask for notifications the first time a status arrives, when they would first matter.
        val statuses by model.statuses.collectAsStateWithLifecycle()
        val askNotify = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}
        LaunchedEffect(statuses.isNotEmpty()) {
            val prefs = getSharedPreferences("statuses", MODE_PRIVATE)
            if (statuses.isNotEmpty() && android.os.Build.VERSION.SDK_INT >= 33 && !prefs.getBoolean("asked", false) &&
                checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                prefs.edit().putBoolean("asked", true).apply()
                askNotify.launch(android.Manifest.permission.POST_NOTIFICATIONS)
            }
        }
        // A file shared from another app goes to the next chat opened.
        LaunchedEffect(chat?.id, share) {
            val s = share
            if (chat != null && s != null) { model.upload(s); shared.value = null }
        }
        if (share != null && chat == null && screen is Screen.Home && page == "home") page = "share"

        BackHandler(enabled = chat != null && info == null) { model.close() }
        Box(Modifier.fillMaxSize()) {
            val view = when {
                screen is Screen.SignIn -> "signin"
                info != null -> "info"
                chat != null -> "chat"
                else -> page
            }
            AnimatedContent(
                targetState = view,
                transitionSpec = {
                    val deeper = depth(targetState) >= depth(initialState)
                    if (deeper) slideInHorizontally { it } togetherWith slideOutHorizontally { -it / 4 }
                    else slideInHorizontally { -it / 4 } togetherWith slideOutHorizontally { it }
                },
                label = "navigation",
            ) { v ->
                when (v) {
                    "signin" -> (screen as? Screen.SignIn)?.let { LinkScreen(it, model::signIn, onBack = if (model.hasSaved) model::cancelLink else null) }
                    "info" -> info?.let { id ->
                        InfoPage(id, model, InfoActions(
                            onBack = { info = null },
                            onMessage = { target -> info = null; page = "home"; if (chat?.id != target) model.open(target) },
                            onFiles = { webFiles() },
                            onPickPhoto = { target, group -> photoFor = target to group; photo.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) },
                            onOpenFile = open,
                        ))
                    }
                    "chat" -> chat?.let { c ->
                        ChatScreen(
                            state = c,
                            memberOf = model::memberOf,
                            roleOf = { model.roleOf(c.id, it) },
                            media = model.media,
                            photo = model.photoOf(c.id),
                            observe = observe,
                            linked = linked,
                            actions = ChatActions(
                                onBack = model::close,
                                onInfo = { info = c.id },
                                onMode = { mode = true },
                                onSend = { model.send(it) },
                                onSendFile = { text, file -> model.send(text, file) },
                                onUpload = { media ->
                                    if (media) gallery.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageAndVideo)) else document.launch(arrayOf("*/*"))
                                },
                                onRetry = model::retry,
                                onOlder = model::loadOlder,
                                onInserted = model::inserted,
                                onCommand = model::command,
                                onPause = model::setPaused,
                                onControl = model::control,
                                onOpenFile = open,
                                onObserve = model::setObserve,
                                onLink = model::showLink,
                                onBrowse = webFiles,
                                onClear = { model.clearChats(setOf(c.id)) },
                                files = model::files,
                                models = model::models,
                            ),
                        )
                    }
                    "new-chat", "share" -> NewChatPage(agents, model, onBack = { page = "home"; shared.value = null },
                        onNewGroup = { page = "new-group" }, onStartAgent = { page = "start-agent" },
                        onOpen = { page = "home"; model.open(it) })
                    "new-group" -> NewGroupPage(agents, model, onBack = { page = "home" }, onCreate = { name, members -> page = "home"; model.createGroup(name, members) })
                    "start-agent" -> StartAgentPage(model, onBack = { page = "home" }, onStart = { machine, provider, folder -> page = "home"; model.startAgent(machine, provider, folder) })
                    "linked" -> LinkedDevicesPage(model, onBack = { page = "home" })
                    "sessions" -> io.github.phantomconartist.mace.ui.SessionsPage(model, onBack = { page = "home" })
                    else -> (screen as? Screen.Home)?.let { s ->
                        HomeScreen(s, agents, model, HomeActions(
                            onFilter = model::setFilter,
                            onRefresh = model::refresh,
                            onOpen = model::open,
                            onInfo = { info = it },
                            onNewChat = { page = "new-chat" },
                            onNewGroup = { page = "new-group" },
                            onStartAgent = { page = "start-agent" },
                            onClear = model::clearChats,
                            onMarkRead = model::markRead,
                            onSignOut = { model.signOut() },
                            onLinked = { page = "linked" },
                            onSessions = { page = "sessions" },
                            onLink = model::showLink,
                        ))
                    }
                }
            }
            SnackbarHost(snack, Modifier.align(Alignment.BottomCenter).navigationBarsPadding().padding(bottom = 72.dp))
        }
        if (mode) chat?.let { c ->
            var policy by remember(c.id) { mutableStateOf<io.github.phantomconartist.mace.data.Policy?>(null) }
            LaunchedEffect(c.id) { policy = runCatching { model.policy(c.id) }.getOrDefault(io.github.phantomconartist.mace.data.Policy(mode = if (c.group) "team" else "reply")) }
            policy?.let { p -> PolicySheet(c.id, rooms[c.id], agents, p, onSave = { model.savePolicy(c.id, it); mode = false }, onDismiss = { mode = false }) }
        }
    }

    private fun depth(view: String) = when (view) {
        "signin", "home" -> 0
        "chat", "new-chat", "share", "linked", "start-agent", "new-group" -> 1
        else -> 2
    }
}
