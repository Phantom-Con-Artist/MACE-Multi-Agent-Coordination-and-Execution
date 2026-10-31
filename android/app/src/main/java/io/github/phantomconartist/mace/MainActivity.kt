package io.github.phantomconartist.mace

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.phantomconartist.mace.ui.ChatsScreen
import io.github.phantomconartist.mace.ui.SignInScreen
import io.github.phantomconartist.mace.ui.theme.MaceTheme

class MainActivity : ComponentActivity() {
    private val model: AppModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            MaceTheme {
                val screen by model.screen.collectAsStateWithLifecycle()
                when (val s = screen) {
                    is Screen.SignIn -> SignInScreen(s, model::signIn)
                    is Screen.Chats -> ChatsScreen(s, model::refresh, { model.signOut() })
                }
            }
        }
    }
}
