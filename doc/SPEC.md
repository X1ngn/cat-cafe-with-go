### 模型调度器 SPEC

#### 1. 系统概述
该调度器负责管理多个Agent的任务分发，使用Redis Streams作为事件队列，实现用户、Agent与Agent之间的无状态、低耦合通信。每个Agent通过配置文件注册自己的监听管道，并从指定的`.md`文件中读取系统提示词，调度器根据用户输入将消息发送至对应Agent的管道，任务按顺序执行。

#### 2. 组件
- **调度器**：负责协调Agent之间的任务，确保任务按顺序进行。
- **Agent**：每个Agent负责特定任务，任务完成后返回空闲状态。每个Agent从配置文件指定的`.md`文件中读取系统提示词。
- **Redis Streams**：作为消息队列，传递Agent之间和用户与Agent之间的消息。
- **YAML配置文件**：定义所有Agent的配置信息，包括Agent名称、管道、执行命令和系统提示词文件路径等。

#### 3. 功能要求
- **无状态通信**：每个Agent和调度器之间的通信无状态，Agent在任务完成后恢复到空闲状态。
- **消息可靠性**：确保每个消息都能成功处理，失败时会有重试机制。
- **顺序任务执行**：任务按顺序执行，避免并发带来的混乱。
- **用户输入函数**：用户指定目标Agent和任务内容，调度器将消息发送至指定Agent的管道。
- **任务配置**：所有Agent及其配置信息通过YAML文件配置。
- **系统提示词**：每个Agent从配置文件中指定的`.md`文件路径读取系统提示词，用于执行任务时的上下文管理。

#### 4. 系统架构
- **Agent管理**：每个Agent有一个独立的管道，注册时通过配置文件指定，调度器读取YAML文件来管理每个Agent的配置信息。
- **消息队列**：Redis Streams用作事件队列，确保消息可靠传递。
  - 任务通过消息发送至目标Agent的管道。
  - 每个Agent在任务执行完后，返回空闲状态。
  - 失败的任务会重新加入队列，进行重试。
- **任务执行**：
  - 用户通过输入指定Agent及任务内容。
  - 调度器根据输入将任务发送至指定Agent的管道。
  - Agent处理完任务后，返回结果并进入空闲状态。
- **系统提示词**：每个Agent的配置中包含一个指向`.md`文件的路径，Agent读取该文件并使用其中的内容作为系统提示词。

#### 5. 配置文件 (YAML 示例)
```yaml
agents:
  - name: "Agent_A"
    pipe: "pipe_A"
    exec_cmd: "invoke.go"
    system_prompt_path: "/path/to/agent_A_prompt.md"
  - name: "Agent_B"
    pipe: "pipe_B"
    exec_cmd: "invoke.go"
    system_prompt_path: "/path/to/agent_B_prompt.md"
  - name: "Agent_C"
    pipe: "pipe_C"
    exec_cmd: "invoke.go"
    system_prompt_path: "/path/to/agent_C_prompt.md"
