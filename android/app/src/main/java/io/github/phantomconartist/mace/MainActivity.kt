package io.github.phantomconartist.mace

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.material3.Surface
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.phantomconartist.mace.ui.ChatScreen
import io.github.phantomconartist.mace.ui.HomeScreen
import io.github.phantomconartist.mace.ui.SignInScreen
import io.github.phantomconartist.mace.ui.theme.MaceTheme

class MainActivity : ComponentActivity() {
    private val model: AppModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            MaceTheme {
                Surface {
                    val screen by model.screen.collectAsStateWithLifecycle()
                    val chat by model.chat.collectAsStateWithLifecycle()
                    val agents by model.agents.collectAsStateWithLifecycle()
                    // The back gesture leaves a chat for the list.
                    BackHandler(enabled = chat != null) { model.close() }
                    AnimatedContent(
                        targetState = chat?.id,
                        transitionSpec = {
                            if (targetState != null) slideInHorizontally { it } togetherWith slideOutHorizontally { -it / 4 }
                            else slideInHorizontally { -it / 4 } togetherWith slideOutHorizontally { it }
                        },
                        label = "navigation",
                    ) { open ->
                        val c = chat
                        when {
                            open != null && c != null -> ChatScreen(
                                state = c,
                                memberOf = model::memberOf,
                                onBack = model::close,
                                onSend = model::send,
                                onRetry = model::retry,
                                onOlder = model::loadOlder,
                                onInserted = model::inserted,
                                onCommand = model::command,
                                onPause = model::setPaused,
                                onControl = model::control,
                                files = model::files,
                                models = model::models,
                                onSendFile = model::send,
                            )
                            else -> when (val s = screen) {
                                is Screen.SignIn -> SignInScreen(s, model::signIn)
                                is Screen.Home -> HomeScreen(s, agents, model::setFilter, model::refresh, model::open, { model.signOut() })
                            }
                        }
                    }
                }
            }
        }
    }
}
