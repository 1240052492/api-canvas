import { newApiClient } from '../utils/new-api-client'
import { nanoid } from 'nanoid'

export interface GenerateImageParams {
  user_id: string
  task_id: string
  model: string
  prompt: string
  estimated_cost: number
}

export interface GenerateImageResult {
  image_url: string
  actual_cost: number
}

export async function generateImage(params: GenerateImageParams): Promise<GenerateImageResult> {
  const { user_id, task_id, model, prompt, estimated_cost } = params

  // Step 1: Pre-deduct
  const preDeductKey = `${user_id}_${task_id}_prededuct`
  let deductionId: string

  try {
    const preDeductResult = await newApiClient.preDeduct({
      user_id,
      amount: estimated_cost,
      idempotency_key: preDeductKey,
      metadata: { task_id, model, prompt }
    })
    deductionId = preDeductResult.deduction_id
  } catch (error) {
    if (error instanceof Error && error.message.includes('INSUFFICIENT_QUOTA')) {
      throw new Error('余额不足，请前往充值')
    }
    throw error
  }

  // Step 2: Call AI generation via New API
  let imageUrl: string
  let actualCost: number

  try {
    const response = await fetch(`${process.env.NEW_API_BASE_URL}/v1/images/generations`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${process.env.NEW_API_SERVICE_TOKEN}`,
        'X-Request-ID': `canvas_${Date.now()}_${nanoid(8)}`,
        'X-Canvas-Task-Id': task_id
      },
      body: JSON.stringify({
        model,
        prompt,
        size: '1024x1024',
        n: 1
      })
    })

    if (!response.ok) {
      throw new Error(`Generation failed: ${response.statusText}`)
    }

    const data = await response.json()
    imageUrl = data.data[0].url
    actualCost = data.usage?.total_cost || estimated_cost

  } catch (error) {
    // Step 3a: Refund on failure
    const refundKey = `${user_id}_${task_id}_refund`
    await newApiClient.refund({
      deduction_id: deductionId,
      idempotency_key: refundKey,
      reason: 'generation_failed'
    })
    throw error
  }

  // Step 3b: Settle with actual cost
  const settleKey = `${user_id}_${task_id}_settle`
  await newApiClient.settle({
    deduction_id: deductionId,
    actual_cost: actualCost,
    idempotency_key: settleKey
  })

  return { image_url: imageUrl, actual_cost: actualCost }
}
