package io.github.phantomconartist.mace

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import io.github.phantomconartist.mace.data.AgentStatus

/**
 * Phone notifications for the status board, shown only while MACE is in the
 * background. They come from the hub's event stream the app already holds,
 * so they stop when Android stops the app; reaching a closed app needs a push
 * service, which MACE does not use yet.
 */
class Notify(private val context: Context) {
    init {
        if (Build.VERSION.SDK_INT >= 26) {
            val channel = NotificationChannel(CHANNEL, "Agent statuses", NotificationManager.IMPORTANCE_DEFAULT)
            channel.description = "An agent needs something its machine does not have."
            context.getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
        }
    }

    val allowed: Boolean
        get() = Build.VERSION.SDK_INT < 33 || context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

    /** A new status, or one still open after an hour. */
    fun status(s: AgentStatus, reminder: Boolean) {
        if (!allowed) return
        val open = Intent(context, MainActivity::class.java)
            .setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP)
            .putExtra(EXTRA_STATUS, s.id)
        val tap = PendingIntent.getActivity(context, s.id.hashCode(), open, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val title = if (reminder) "${s.forName} still needs help" else "${s.forName} needs something"
        val n = NotificationCompat.Builder(context, CHANNEL)
            .setSmallIcon(R.drawable.ic_launcher_foreground)
            .setContentTitle(title)
            .setContentText(s.text)
            .setStyle(NotificationCompat.BigTextStyle().bigText("${s.text}\n${s.roomName} · ${s.machine}"))
            .setContentIntent(tap)
            .setAutoCancel(true)
            .build()
        runCatching { NotificationManagerCompat.from(context).notify(s.id.hashCode(), n) }
    }

    companion object {
        const val CHANNEL = "status"
        const val EXTRA_STATUS = "status"
    }
}
