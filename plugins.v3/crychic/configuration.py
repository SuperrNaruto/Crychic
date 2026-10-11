"""复用宿主 Vuetify 表单的配置引导，不写通知设置或展示凭据。"""

from app.sdk.services import NotificationHelper

CHANNEL_TYPE = "crychic"
SINGLE_CHANNEL = 1
GRID_COLUMNS = 12
HALF_GRID_COLUMNS = 6
MIN_TOPIC_ID = 0
DEFAULTS = dict(enabled=False, channel_id="", default_target="", topic_id=MIN_TOPIC_ID)


def guide_step(title, text):
    """生成配置与详情共用的纯文本步骤，长标识符可在窄屏换行。"""
    return dict(component="VAlert", props={"title": title, "variant": "tonal",
                "icon": False, "class": "mb-4", "style": {"overflowWrap": "anywhere", "whiteSpace": "pre-line"}},
                text=text)


def setup_steps():
    """说明原生渠道、接入参数与机器人边界，只给引导不代做操作。"""
    return [guide_step("1. 在 MoviePilot 创建通知渠道",
            "打开 MoviePilot 的通知设置，新增「自定义」渠道，自定义类型填 crychic，名称自行指定。"
            "启用渠道并选择要接收的通知类型，发送时段和用户范围继续在 MoviePilot 中配置。"),
            guide_step("2. 填写 Crychic 接入参数",
            "在插件配置表单中选择通知渠道，填写默认目标和话题 ID，开启接入后保存。"
            "只有一条 crychic 渠道时可以自动选择；多条时请选择具体名称。"
            "个人通知使用 MoviePilot 已绑定的 Telegram 用户目标，默认目标用于获准的默认通知范围。"),
            guide_step("3. 核对机器人配置与新通知",
            "Go 机器人需配置以下环境变量。\n"
            "CRYCHIC_MOVIEPILOT_URL\n"
            "CRYCHIC_MOVIEPILOT_API_KEY\n"
            "CRYCHIC_TELEGRAM_TOKEN\n"
            "CRYCHIC_TELEGRAM_ALLOWED_USERS\n\n"
            "已有部署保留原配置，"
            "API key 与 Bot token 只填写在 Go 端。\n\n"
            "保存后在 Telegram 核对一条新产生且已启用类型的 MP 通知。"
            "需要排查时，先核对 MP 的渠道启用、通知类型、发送时段和个人绑定，再检查 Go 连接与白名单。"
            "首次接入从新通知开始，原历史通知保持原状。")]


def input_field(model, label, hint, *, component="VTextField", **props):
    """为原有配置字段提供常驻提示，保留宿主原生输入和绑定。"""
    return dict(component=component, props=dict(model=model, label=label, hint=hint,
                persistentHint=True, **props))


def notification_choices():
    """只读取 crychic 原生渠道元数据，失败与空目录明确区分。"""
    try:
        channels = NotificationHelper().get_configs().values()
        return [dict(title=channel.name, value=channel.id) for channel in channels
                if channel.type == CHANNEL_TYPE and channel.id]
    except Exception:
        return None


def channel_input():
    """按名称选择稳定 ID，打开页面不替用户改写已保存选择。"""
    choices = notification_choices()
    if choices is None:
        hint = "通知渠道暂时读取失败，恢复后重新打开本页；已保存的选择会保留。"
    elif not choices:
        hint = "尚未找到 crychic 渠道，完成第 1 步后重新打开本页。"
    elif len(choices) == SINGLE_CHANNEL:
        hint = "当前只有一条 crychic 渠道，保留自动选择即可，也可按名称指定。"
    else:
        hint = "当前存在多条 crychic 渠道，请选择与 MoviePilot 通知设置一致的名称。"
    automatic = dict(title="自动选择（仅一条渠道时）", value="",
                     props=dict(disabled=len(choices or []) != SINGLE_CHANNEL))
    return input_field("channel_id", "通知渠道", hint, component="VSelect",
                       items=[automatic] + (choices or []), itemTitle="title", itemValue="value",
                       disabled=not choices)


def configuration_form():
    """将步骤放在原生表单旁，桌面两列、窄屏按原顺序单列。"""
    before, parameters, after = setup_steps()
    fields = [input_field("enabled", "启用接入", "通知渠道和 Go 机器人准备好后开启，保存后生效。",
                          component="VSwitch"),
              channel_input(),
              input_field("default_target", "默认 Telegram 目标",
                          "个人填正用户 ID，群组或频道填负 Chat ID，也支持 @channelusername；"
                          "个人绑定目标优先。", placeholder="123456789"),
              input_field("topic_id", "默认目标的话题 ID",
                          "仅负数群组或频道 Chat ID 支持话题。没有话题或使用 @名称时保留 0。",
                          type="number", min=MIN_TOPIC_ID)]
    columns = [dict(component="VCol", props=dict(cols=GRID_COLUMNS, md=HALF_GRID_COLUMNS), content=[field])
               for field in fields]
    content = [before, parameters, dict(component="VRow", content=columns), after]
    return [dict(component="VForm", content=content)], dict(DEFAULTS)
